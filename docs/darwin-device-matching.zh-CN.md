# Darwin：元数据枚举与精确设备打开

此候选把元数据枚举与设备打开分开，避免在找blink(1)时先打开无关键盘、触控板等HID。此前 `iterateDevices` 先设置NULL匹配，再调用 `IOHIDManagerOpen`，之后SDK才按VID/PID过滤；`DeviceInfo.Open` 也沿用这个全量manager，并保留到设备关闭。Apple SDK对ManagerOpen的说明是会打开当前及将来枚举到的设备，因此晚于它的Go层过滤不能缩小打开范围。

本轮没有改变公开API，也没有改变 `go 1.13` 声明：

| 入口 | Darwin行为 |
| --- | --- |
| `Devices()` / `ListAllDevices` | 仍可获取全量HID元数据；manager只Create、SetMatching、CopyDevices、Release，不再ManagerOpen/Close |
| `FindDevices(vendor, product)` | 先在IOKit匹配字典中设置两个精确数字，再取元数据；返回前仍做原来的精确相等校验 |
| `DeviceInfo.Open()` | 先核对Info与现有Path编码的VID/PID，再设置非NULL精确匹配，仅对原Path完全命中的单个设备调用 `IOHIDDeviceOpen` |

`FindDevices` 的0仍表示精确的ID=0，不是通配符。通用全量枚举依然可用；需要限定设备类型的调用者应使用 `FindDevices`，不能指望 `ListAllDevices(cond)` 的Go谓词被自动翻译成原生匹配。其他平台保持原有枚举加精确过滤方式。

Darwin既有Path格式为 `transport_vendor_product_location`。Open从末尾三个固定宽度十六进制字段解析ID，兼容仅填写Path的历史 `DeviceInfo`；transport可包含下划线或为空，最后仍比较完整原Path。带有非零ID字段的Info必须与Path一致。格式非法、字段不一致或分配失败时直接失败，绝不回退到NULL/全量匹配。

匹配字典拥有两个CFNumber；临时CFNumber引用在放入字典后释放，字典使用完释放。manager未被Open，因此清理只释放其引用；成功Open的目标仍被CFRetain，设备关闭时先DeviceClose、释放设备引用，再释放保留的manager。原有枚举锁与每个活句柄的完整原生I/O锁保留，没有新增异步输入回调或改变关闭超时承诺。

## 默认测试不访问主机HID

真实枚举测试改为显式启用，默认会说明跳过；这不算硬件覆盖通过：

```sh
# 默认仅运行软件、CoreFoundation与closed-handle测试。
GID_TEST_ENUMERATE= GID_TEST_DEVICE_PATH= go test -race ./...

# 以下是真实主机枚举，须有意选择；不是普通软件检查。
GID_TEST_ENUMERATE=1 go test -run 'TestThreadedDevices|TestListAll' -v

# 仅在明确指定设备路径后才允许实际Open。
GID_TEST_DEVICE_PATH='明确路径' go test -run TestOpenSelectedDevice -v
```

新增软件检查覆盖实际CFDictionary中两个CFNumber的类型和值（含0和65535）、模拟分配失败不退化为通配、Path-only/空transport/下划线transport/非法Path/ID不一致，以及精确0值的Go过滤。不存在Path的Open测试先在纯Go层失败，不创建IOHIDManager。原生测试二进制还可用 `nm -u` 核对：应保留选定设备的DeviceOpen/Close引用，不含ManagerOpen/Close引用。

这些检查不能替代本机新可执行文件的USB、TCC权限归因及控制/关闭验证。当前Mac的真机验证由集成验收另行记录；没有承诺旧macOS、其他操作系统USB或所有HID类型均已测试。枚举API仍不带error返回值，因此枚举阶段的资源失败沿用空结果行为，不应将空列表单独视为“设备一定不在”。

## 实现依据

- 本机Apple SDK的 `IOKit/hid/IOHIDManager.h`：ManagerOpen会对枚举设备传播打开，NULL匹配枚举全部设备；CopyDevices用于取得已枚举设备集合。
- 现有[Dolmen/HIDAPI的hid_enumerate](https://github.com/dolmen-go/hid/blob/27ac09a/hidapi/mac/hid.c#L393)使用SetMatching、CopyDevices读取设备信息，没有在这条枚举路径调用ManagerOpen。此参考仅支撑枚举方式，不表示照搬其0值通配语义，也不证明其其他入口没有广域打开。
