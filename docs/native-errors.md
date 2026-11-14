# 原生错误码与兼容错误文本

`NativeError` 是新增的错误包装类型，不改变 `Device` 接口或Go1.13声明。

```go
type NativeError struct {
    Platform string
    Code     uint64
    Err      error
}
```

存在原始cause时，`Error()` 原样返回 `Err.Error()`，不会自动加平台、十进制错误码或新前缀。`Unwrap()` 返回该cause，保留标准的 `errors.Is` / `errors.As` 链。无cause时使用通用文字 `native error`；它不自行解释错误含义。

调用方可以只依赖一个小接口，不要求导入新类型：

```go
var native interface {
    NativeCode() uint64
    NativePlatform() string
}
if errors.As(err, &native) {
    fmt.Printf("platform=%s native_code=0x%x\n",
        native.NativePlatform(), native.NativeCode())
}
```

上述示例仅使用标准库 `errors` / `fmt`。即使底层版本或某条错误路径没有这些方法，`errors.As` 也只会返回false；不能把方法缺失当成原生成功码0。

## 当前接入范围

本轮只接入Darwin的 `ioReturnToErr`：

- `kIOReturnSuccess` 仍返回真正的nil，不构造带nil指针的error接口。
- 现有每个IOKit映射的文字保持不变，平台标记为 `darwin`。
- IOReturn原先经有符号32位值传入，记录时使用 `uint64(uint32(code))` 保留低32位，避免符号扩展。例如 `0xe00002c0` 仍是该无符号位模式，而不是一个扩展到64位的负数表示。
- 未列入映射的错误仍显示 `unknown error`，同时保留其真实原始码；不会给未知码编造分类。

例如原来的 `no such device` 仍是相同文本，但调用方现在可以同时取得 `darwin` 与 `0xe00002c0`。上层SDK使用 `%w` 包装时不丢失该信息。

关闭前检查、设备未找到等非原生错误并不一定包含该类型。Windows现有 `syscall.Errno` 和Linux错误路径没有在本轮被改写，也不会凭空补上native code。对外JSON、重试策略、错误分类和可恢复性由应用层独立定义；本类型不提供 `Retryable` 或取消/超时保证。

## 软件验证与边界

通用测试在CGO=0下验证包装后的traits、64位值保留、消息与cause身份，不调用USB。Darwin测试使用本机IOKit定义对应的固定码和旧消息，覆盖全部已有映射、未知值、无符号表示和success=nil；测试文件不导入C，也不枚举或打开设备。

这些检查不代替真实拔插、权限、独占或非响应设备测试，也不意味着其他平台已经提供同一原生诊断层。
