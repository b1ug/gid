# Linux 精确发现与权限边界

`FindDevices(vendor, product)` 在 Linux/cgo 后端将原参数传给内嵌
`hid_enumerate`。该函数在尝试 `libusb_open` 查询 serial、厂商和产品字符串
之前先按 VID/PID 缩小 HID 候选，避免为非匹配 HID 做这一轮打开。SDK 的
`FindDevices(0x27b8, 0x01ed)` 自动使用该路径，不需要新增 API。

公开返回值的历史语义保持不变：`FindDevices` 最后仍以 Go 精确比较两个 ID。
例如 `FindDevices(0, 0x01ed)` 只返回 VendorID 真正为零的匹配元数据；它并不
表示任意厂商。native HIDAPI 中零是 wildcard，因此这类请求在 native 层
取得较宽候选，再由 Go 精确筛选。`Devices()` 继续全量发现，基于它的通用
`ListAllDevices`、`ListFirstDevice`、`FindDevicesByProduct` 和 Linux `ByPath`
也保留原有发现范围。枚举顺序、重复接口及元数据字段保留。

## 实际权限与尚存边界

内嵌 `hidapi_linux.h` 的 `libusb_open` 最终以 `O_RDWR` 打开 USB 设备节点。
字符串查询是 USB descriptor control 读取；默认枚举没有开启
`INVASIVE_GET_USAGE`，不会因此 detach/claim 接口，也不会创建 HID input
read_thread。打开成功的字符串查询句柄会关闭；无权限打开时，设备仍可返回
但可选字符串为空。Go producer 保持枚举锁与 native 元数据释放顺序。

仅下推参数不能消除所有无关节点访问。libusb 先发现 USB 设备并缓存 descriptor：
现代正常挂载 sysfs 时读取只读元数据；内核较老或 sysfs 不可用时，
`initialize_device` 还可能在 HID/VID 筛选之前尝试 `O_RDWR` 查询 active config。
本次不修改或升级内嵌 libusb，不宣称旧路径已经消除，也不要求调用者扩大权限。
无法读取或没有结果不代表硬件不存在；通道 API 仍不能报告枚举错误。

默认初始化的 netlink 线程监听 `NETLINK_KOBJECT_UEVENT` 热插拔元数据，
与 HID 输入报告不同。显式选定 `DeviceInfo.Open()` 则在完整 path 命中后才
打开、可能 detach/claim 该接口并启动 interrupt input 线程；这属于所选设备
的数据 I/O 生命周期。

Windows 本次没有改变。其发现临时句柄使用 `CreateFile` 的
`desiredAccess=0`、share READ|WRITE，只查询属性、字符串和 preparsed/caps，
不申请 `GENERIC_READ/WRITE`，也不读取 input/feature 报告。零访问仍经过设备
对象和驱动权限检查，不等于完全不接触设备或保证获准。显式选定设备的 Open
才申请通用读写。[Microsoft 访问语义](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)
和 [HID 元数据与报告操作](https://learn.microsoft.com/en-us/windows-hardware/drivers/hid/opening-hid-collections)
分别描述这两个阶段；内嵌 Linux 的具体行为以本库固定源码为准。

## 无硬件软件验证

```sh
GOWORK=off GID_TEST_ENUMERATE=0 GID_TEST_DEVICE_PATH= go test -run '^TestLinuxDiscovery' -v .
python3 ci/check-linux-discovery.py
```

第一层从实际 Go 源的发现入口及最终过滤器解析函数，编译在临时独立 Go 程序中，
用软件 C 边界替身验证原始参数到达、三种零组合、无匹配、最大 ID、全量
`Devices()`、重复顺序与 native 列表释放。它不把生产 native 函数替换成可变全局
hook。构建选择覆盖 Linux/Android cgo、CGO0 fallback、Darwin、iOS、Windows
与其他平台。

第二层需要 Python 3 和 C 编译器，将实际内嵌 `hid_enumerate` 与
`hid_free_enumeration` 函数及默认宏片段编译执行；所有 libusb 入口、设备、
初始化、descriptor、字符串均为软件假实现。10 个情形验证 VID/PID 条件确实位于
字符串打开前、非 HID 不打开、复合接口重复顺序、每个成功句柄关闭、失败打开
保留可选空字段、config fallback 与 init/list 失败，以及元数据分配释放平衡。
这不证明真实内核访问模式、udev 规则或 Linux USB 驱动行为。

软件测试与 Mac 编译不能算作 Linux 原生或 USB 验收；后续仍需普通用户 Linux
原生检查，并在授权的真实 blink(1) 环境跟踪普通 sysfs 与同机其他 HID 的打开范围。
旧/缺失 sysfs 路径需要单独验证。Go module 保持 `go 1.13`，无新增依赖。
