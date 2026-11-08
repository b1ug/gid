# GID 当前候选：改动与验证边界

记录日期：2026-11-09。本文区分源码改动、软件测试和真实 USB 验证，后续结果应按实际提交追加。

## 提交与审查边界

[PR #2](https://github.com/b1ug/gid/pull/2) 停在 `f520a7e8cdb8567ebeb14239cbc3fe794b7bdffb`，等待审查。以下改动在独立的 `validation/runtime-gid` 候选分支推进，未因此合并、发布或更新该 PR。

| 提交 | 候选改动 |
| --- | --- |
| `0b4b191` | 将 Windows 整数 HANDLE 到原生类型的转换放在 C 包装中，避免 Go 把它当指针 |
| `c0be9f8` | Linux 为整个 I/O 和错误文本复制持有句柄租约；关闭前拒绝新操作；输入读取分片检查关闭 |
| `8bb285b` | 记录嵌入依赖的已知来源、未知上游版本和可离线重跑的指纹审计 |
| `da440ba` | Darwin 错误增加原始 IOKit 码和平台 traits，保持原来的错误文本 |
| `6d51cae` | Windows 请求取消、等待完成后关闭；异步报告缓冲、事件和 OVERLAPPED 由 C 内存持有 |
| `0e9d5af` | 修正上述 Windows 异步调用两处 `C.PVOID` 参数类型 |
| `1ea70d8` | Windows 枚举检查两阶段查询和 NUL 边界，以 C 原生结构大小填写详情头 |

本轮功能源码快照是 `1ea70d86ea1625558012a06f0cf3c0e0a4a4b576`。后续纯文档提交不会让已有 CI 自动覆盖新源码。

## 关闭与并发约定

| 平台 | 当前实现 | 仍不能承诺的事情 |
| --- | --- | --- |
| macOS | 同一互斥锁覆盖完整同步 I/O 与关闭；重复关闭安全；关闭后操作报错；无异步移除回调 | 原生调用的硬截止时间、无需重新打开的拔插恢复 |
| Linux | 共享租约允许读写并行；关闭先发布信号，再等待所有租约并释放一次；租约包含原生错误文本复制 | 特性报告调用、原生清理或读线程 join 必定在某时间内返回 |
| Windows | 关闭先阻止新 I/O，再请求 `CancelIoEx`，最后等待所有操作并关闭一次；晚于取消快照发出的操作再次取消自身 | 驱动必然响应取消、同步 `HidD_SetFeature` 可取消、固定关闭耗时 |

Linux 的输入读取内部使用 100 ms 的 `hid_read_timeout` 切片，空切片不会作为成功的空报告返回。它保留阻塞读取语义，也让关闭信号有机会被读取循环观察。嵌入 C 实现使用的时钟和清理行为意味着 **100 ms 不是关闭上限**。

Windows 的异步载荷、OVERLAPPED 和事件必须保留到 `GetOverlappedResult` 已收集完成结果。`CancelIoEx` 只是取消请求，不能据此提前关闭 HANDLE 或释放内存。原生调用错误在同一次 C 包装调用中捕获，避免稍后的调用覆盖线程错误码。

以上保护的是资源生命周期。它不会将“写命令、读响应”两次调用合成事务；上层协议仍应由一个设备所有者串行执行。它也不提供统一的 `context.Context` / timeout API。

## 错误与枚举边界

- Darwin `NativeError` 保留原错误字符串、cause 和低 32 位 IOReturn，未知码保留真实值而不编造分类。Windows `syscall.Errno`、Linux 文本错误没有被统一改写。详见[原生错误说明](native-errors.md)。
- Windows 枚举详情、设备类和驱动属性查询均检查探测与填充结果；尺寸超过 1 MiB、结果增长、非法偏移或无终止 NUL 时返回空结果。同步调用借用零初始化 Go 字节缓冲，字符串复制有界。
- `SP_DEVICE_INTERFACE_DETAIL_DATA_A.cbSize` 使用 C 的 `sizeof`；ANSI x86 的 5 字节结构不能用 4 字节指针大小替代。相关测试须在原生 Windows CGO 下执行，详见[Windows 枚举与 ABI 说明](windows-enumeration-validation.md)。
- 依赖来源审计并未把嵌入 HIDAPI/libusb 自动升级为最新版本，也没有推断未知的上游 SHA。详见[依赖来源记录](dependency-sources.md)。

## 已有 CI 证据及失败记录

这些运行来自 nb1 的组合工作流，固定 CLI、SDK 和 GID 版本后验证集成。绿色结果只属于表中的 GID 版本。

| 运行 | GID 源码 | 结果与含义 |
| --- | --- | --- |
| [42511173393](https://github.com/b1ug/nb1/actions/runs/42511173393) | `0b4b191` | 7/7 通过，包含原生 Windows、Mac/Linux 和旧 Go 源码检查；尚无后来的 Linux/Windows 生命周期改动 |
| [42511900040](https://github.com/b1ug/nb1/actions/runs/42511900040) | `c0be9f8` | 7/7 通过，包含 Linux 生命周期实现；不覆盖后来的 Windows 取消实现和枚举修复 |
| [42514864088](https://github.com/b1ug/nb1/actions/runs/42514864088) | `6d51cae` | Windows 编译失败：两个 `unsafe.Pointer` 参数需要转换为 `C.PVOID`；没有进入 Windows 原生取消测试。其它 6 项通过 |

`0e9d5af` 已修正失败的两个参数；`1ea70d8` 加入枚举修复。**截至本记录，最新候选的原生 Windows CI 尚待完成，不能沿用前两轮绿色结果。**

Windows 原生取消测试使用命名管道制造实际 pending WriteFile，再验证取消、完成收集和关闭顺序。它会执行 Win32 I/O，但不模拟 HID IOCTL，也不需要或验证 USB 设备。通用 Go 租约屏障测试和字符串查询回调测试能在 Mac 上运行；它们不能替代 Windows CGO/ABI 执行。

本轮在 Go 1.27.3、darwin/arm64、CGO=1 下重跑以下检查，全部通过；限定测试名避免枚举或打开设备：

```sh
GOWORK=off go test -race -count=30 -timeout=2m \
  -run '^(TestLinuxLifetime|TestLinuxPollInput|TestWindowsLifetime|TestWindowsCString|TestNativeError)' .
GOWORK=off go test -race -count=30 -timeout=2m -run '^TestDarwinNative' .
GOWORK=off go vet ./...
```

Go 1.13.15 检查保留的是 GID 的源码兼容下限。当前 Go 在当前 CI 系统上编译成功，与旧 Windows、旧 macOS 或旧 Linux 的运行时兼容是不同结论。Windows/386 的拟增检查是在 64 位 Windows 上通过 WOW64 运行 32 位进程，不等同于旧 32 位操作系统支持。

## 当前仍待验证

1. 用最新提交重跑原生 Windows amd64 构建、race、命名管道取消测试和 vet；执行 Windows/386 CGO 结构布局与普通测试。
2. 用 Linux/Windows 实体 HID 验证读取阻塞、写入、设备移除、关闭与重新打开，不把无 USB 的 CI 当成真机证据。
3. 当前已连接的 Mac + blink(1) mk2 可以验证该组合；物理拔插、非响应设备及更老系统仍需各自记录。没有 blink(1) mk3 真机，不能宣布 mk3 已验证。
4. 继续将通用传输层、blink(1) SDK 的命令事务与 daemon 的队列/重试策略分开。GID 不因提供原始错误码就自动重试任意 HID 写入。
