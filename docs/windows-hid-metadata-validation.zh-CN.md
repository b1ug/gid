# Windows HidD 元数据：失败输出与字段污染修复

## 问题与官方契约

这是 `ByPath` 的 HidD 元数据路径，与之前 SetupAPI 的路径/注册表字符串边界修复不同。

原实现把同一个 `[]uint16` 缓冲依次传给 manufacturer、product、serial 查询，每次都忽略 BOOLEAN 结果后解码。如果后一次失败且没有替换内容，就会把前一字段作为后一字段，例如 serial 失败后得到 `blink(1) mk2`，或 product 和 serial 均失败后得到 manufacturer `ThingM`。污染发生在同一次 `ByPath` 内，不是复用上一个设备的缓冲。

微软分别规定这三个调用只有返回 TRUE 才表示取得完整的 NUL 终止宽字符串；FALSE 不能作为有效元数据消费。BufferLength 的单位是字节。每次分配 256 个 UTF-16 单元对应 Windows 的 512 字节，小于文档的 4093 字节上限。USB 字符串最多 126 个宽字符，**不包括**终止 NUL，因此该空间足够；这不是说所有 HID 设备的字符串都只有同一个长度。

依据：[HidD_GetManufacturerString](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_getmanufacturerstring)、[HidD_GetProductString](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_getproductstring)、[HidD_GetSerialNumberString](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_getserialnumberstring)。

`HIDD_ATTRIBUTES` 原来是每次新建的零值结构，所以失败且完全未写时得到零，而不是上个设备的 VID/PID。但是原实现也没有检查成功位，仍可能消费失败调用留下的部分输出。微软仅保证 TRUE 代表成功；失败时应拒绝这一设备信息结果。[HidD_GetAttributes 契约](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_getattributes)

## 最小修复

- 三个可选字符串各自使用独立的零初始化 UTF-16 缓冲。FALSE 返回空值；TRUE 仍须在分配范围内找到 NUL 才解码，支持正常 Unicode。
- BufferLength 使用 `len(buffer) * C.sizeof_WCHAR`，不再把宽字符单元数直接作为字节数。
- 属性查询失败时 `ByPath` 返回明确 error，不返回可被筛选器消费的部分 DeviceInfo。原生包装在同一次 C 调用中捕获失败状态；调用前清除旧 last-error，未提供错误码时使用通用失败码。属性文档没有保证每种失败都提供专门错误码，因此该码不表示额外的可恢复性分类。
- 属性成功而字符串缺失仍允许返回 DeviceInfo，保留通用 HID 库的可选 metadata 语义。不新增公开 API、设备 profile、mk3 放行或写入重试。

## 对 blink 选择与能力门控的实际影响

当前 SDK 的 `IsBlink1Device` 只检查 VID/PID。runtime 后端在此基础上检查唯一 serial、bcdDevice=2，打开后读取固件并要求 2xx；它尚未使用更严格的 `IdentifyDeviceProfile` 元数据一致性帮助函数。

因此，常见的“product 成功、serial 失败”会把 serial 变成 `blink(1) mk2`，已配置的 `20005601` 无法匹配，表现为 offline；带空格/括号的伪 serial 也过不了 runtime 的 serial token 校验。不能据此声称通常会控制错误设备。

但是，如果 product 和 serial 都失败，`ThingM` 这样的 manufacturer 是合法 token；若用户根据错误发现信息配置它，唯一 serial 检查本身不能识别这个值是回退污染。按错误身份命名的协作锁、重连和诊断也不可靠。原生 Windows 的设备占用规则仍独立存在，此缺陷不表示绕过了原生独占。修复后缺失的 serial 为空，不能伪装成上一字段；既有显式 serial 不会自动改选别的设备。

更严格的 runtime profile 一致性校验可另行评估，不能借本修复悄悄改变上层兼容范围。元数据失败原因的诊断也可单独扩展。

## 软件验证与剩余范围

共享纯 Go callback 用例覆盖：product/serial 失败不复用 manufacturer，serial 失败不变为 product，FALSE 即使写入了貌似合法 serial 也必须返回空，属性错误不消费部分 VID/PID，以及 Unicode、空字符串、无 NUL 和末尾 NUL 边界。

在隔离工作树中临时恢复“共享缓冲 + 忽略 BOOLEAN”的旧行为，上述回归按预期失败；随后恢复修复代码。反向结果保留于 `audit/legacy-regression.log`，它不是 Windows 真机失败记录。

本机 Go 1.27.3、darwin/arm64、CGO=1 下的这些无硬件用例，race 重复 30 次、Go 1.13 语言检查、build、vet 通过。它们不枚举或打开设备。

Windows CGO 专属测试包括 C `sizeof(WCHAR)` 算出的 512 字节长度，以及对无效 HANDLE 的属性查询失败。[集成运行42517136819](https://github.com/b1ug/nb1/actions/runs/42517136819)固定GID `7d9978d` 后，Windows amd64原生race/vet与386/WOW64普通测试均通过，整轮8/8。这些无USB软件检查不代表Windows实体设备或热拔插验证完成。
