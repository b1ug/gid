# GID 内嵌依赖来源与离线审计

记录日期：2026年11月9日。审查基于GID本地提交 `c0be9f8`，不修改现有Go/C实现，不升级整个vendor，也不把候选后端改成正式依赖。

## 三类来源分开记录

`go.mod` 声明Go1.13，没有直接 `require`。这不等于“没有依赖”：Linux把HIDAPI/libusb源码合并进了 `hidapi_linux.h`，`wchar.go` 是内嵌的字符转换代码；Darwin和Windows绑定操作系统API。`go get -u` 不会替换这些复制进来的C源码。

| 平台 | 本仓库实际路径 | 现有构建规则 |
| --- | --- | --- |
| Darwin | `gid_darwin.go` | 系统CoreFoundation、IOKit framework；有本地库搜索目录，但没有链接外部HIDAPI/libusb的规则 |
| Windows | `gid_windows.go` | 系统Setupapi、hid库和Windows SDK头文件 |
| Linux | `gid_linux.go` + `hidapi_linux.h` | 内嵌libusb/HIDAPI，POSIX与系统C环境；linux且非android时链接rt。没有使用外部libusb的pkg-config规则 |

这些原生路径需要CGO。CGO关闭时可选择stub，不计作原生HID验证。新增Python审计工具只是开发工具，不改变库的Go1.13声明，也不要求安装额外Go工具依赖。

## 已确认的来源链

1. 历史 `nb1/hid` 现在保存在 [b1ug/nb1dev](https://github.com/b1ug/nb1dev)。`17af24c` 的提交说明明确从boombuler/hid fork；当时的 `hid/hid_darwin.go` 与boombuler比较提交 `6a2dff8` 逐字节一致，但提交说明没有固定所选上游ref。
2. `nb1dev/d2cc764` 明确迁入dolmen-go/hid的Linux实现。其 `hid/hidapi_linux.h` 与当前GID文件逐字节一致。
3. GID `19cb423`（2023-09-19）以“Migrate from nb1/hid”导入C合并头、wchar及旧MIT说明；`39cc55c`将wchar的包名从hid改为gid。当前头文件自导入后内容未变，wchar自导入后仅该包名变化。
4. 已在临时副本中**实际离线重建**dolmen-go/hid固定提交 `27ac09ac07a3d215b5f360726ecff13b50196fde`：`go run ./internal/gen.go` 生成的Linux头与它自己跟踪的头文件相同；将所有字节序列 `#line ` 替换为 `// line ` 后，与当前GID头完全一致。
5. 同一dolmen快照的wchar文件，只需包名hid→gid，以及两处注释拼写vendored→vendorized，就与当前GID文件完全相同。原始gowchar提交未写在头部，因此独立上游revision仍记为未知。

这给出了可复现的**供应快照与变换**，并不能证明原导入者当时选择的git ref正是27ac09a。独立的HIDAPI/libusb原始提交也尚未确认，不能由供应快照或版本宏反推成某个官方发布版。

## 当前指纹

完整机器清单为 [upstream-sources.json](../upstream-sources.json)，记录字节数、行数、SHA256、23个内嵌源路径标记、来源说明和未知项。

| 文件 | 字节数 / 行数 | SHA256 |
| --- | --- | --- |
| `hidapi_linux.h` | 539853 / 16370 | `ff4a7492e300f05e70a6331f3dc3de9da17a822895b634eee9d6880ff79f6102` |
| `wchar.go` | 4822 / 228 | `899dff588a40ee5de9ad0a6a3690280c171b5c35afcaa46207f11be23fbd3719` |
| `LICENSE` | 1076 / 21 | `a428d1101807989e11ae625f4316d360a27b2a922f4f6cb6ce5981e8cfeafe93` |
| `LICENSE-old` | 1085 / 21 | `36d2e94c565d51a5146d729e6be6a3d26168d473d97c94011b13700e82c622e4` |

未变换的供应头SHA256为 `5f8ca3707bf8ae9992fb78d6a160394a2f75e24298ed92f2ddc65633f5018a86`。供应生成器git blob为 `aa9451459aa72fa9539568eb9659daeb68cddb30`。这些值标识已比较的具体内容，不替代代码差异审阅。

内嵌libusb部分可读到MAJOR=1、MINOR=0、MICRO=21；NANO首个定义11182，后面还有受 `#ifndef` 保护的0值fallback。它说明头内的宏，不证明全部合并内容等于官方libusb1.0.21 release。此旧HIDAPI部分没有足以确定独立release的版本宏。

## 现存说明的范围

| 来源 | 本仓库实际保留位置 | 记录到的事实 |
| --- | --- | --- |
| GID自身 | `LICENSE` | MIT，2023 blink(1) user group |
| boombuler/hid | `LICENSE-old`，README来源链接 | MIT，2014 Florian Sundermann；旧license文件与boombuler比较提交中的LICENSE逐字节一致 |
| dolmen供应链 | README来源链接；导入历史；复制进来的下列文件头 | 没有单独复制其 `LICENSE.md`；具体C/字符转换来源在各自文件中标注 |
| libusb | `hidapi_linux.h` 多个源段头，包括第2–22行等 | 多次保留版权持有人与LGPL2.1或更新版的说明；GID没有独立 `COPYING` 文件 |
| HIDAPI | `hidapi_linux.h` 第14465行和第14516行起的源段头 | 头部列出GPLv3、BSD-style或原始HIDAPI许可选项，README链接BSD选项；单独的上游license文件没有随GID复制 |
| gowchar | `wchar.go` 第1–7行 | 保留来源链接和3-clause BSD许可链接；未固定原始revision，没有单独gowchar LICENSE副本 |

这里只记录所见文件与归属范围，不把最外层MIT文本当成每个内嵌源段的说明，也不从此推导最终程序的整体许可结论。

## 离线检查

在仓库内执行：

```bash
python3 tools/audit-upstream.py --check > upstream-audit.json
```

默认JSON同时输出原始来源清单、每个文件的预期/实际指纹与状态，以及实际提取的libusb宏。工具只读文件，不调用git、Go或C，不访问网络，不下载、生成、改写或更新源文件。

- 未传 `--check`：即使发现变化也完成观察并退出0，请读取 `ok` 和各文件状态。
- 传 `--check`：任一文件变化、缺失、不可读或解析到仓库外部，退出1。
- 无法读取清单或清单格式错误：退出2。
- `--root /path/to/source-copy` 可核对一份受控副本，默认不改变清单位置或期望值。

SHA检查发现的是相对已审阅清单的变化；不会证明硬件通过、某个上游release身份，或自动证明整个目录树都未变。

## 可复现的后续升级步骤

1. 先保存现版本的离线审计与原生/设备基线。选定具体上游tag和完整commit，记录要解决的实际问题；不要用动态HEAD作为不变的来源标识。
2. 在独立工作树和临时供应副本中准备原始C源码、配置头、生成器与所选快照的notice/license文件。现有供应生成器只用标准库；执行前固定其源码。不要在当前GID目录直接运行上游的网络升级脚本。
3. 复现旧内容时，在已固定的dolmen副本运行 `GOWORK=off GOPROXY=off GOSUMDB=off go run ./internal/gen.go`，然后在临时副本比较 `generated.replace(b'#line ', b'// line ')` 的SHA。这个既有操作已有离线重建证据。新上游版本若改变头文件结构、平台宏或API，须分别修改并审阅生成流程，不能假定此变换始终够用。
4. 审阅真正的Go/C差异、线程/取消/报文语义及现存notice变化，再更新清单中的精确来源和指纹。只重算SHA不能代替来源证明或差异审阅。
5. 使用同一conformance矩阵对照现后端与候选；保持上层SDK/CLI导入契约，拿到收益与兼容证据后再决定迁移。库版本发布与PR合并另行处理，不由审计工具自动执行。

## 升级需要复用的验证矩阵

| 层次 | 软件验证 | 设备验证与边界 |
| --- | --- | --- |
| Linux amd64/arm64 | CGO=1原生build/test/race/vet；有限读取与Close资源寿命；C层检测单独记录 | 权限/独占、feature与真实input设备、拔插、取消和故障等待。仅blink feature成功不能代替通用input验收 |
| Darwin arm64/Intel | CGO=1原生build/test/race/vet；工具链支持时另跑cgocheck2 | CF对象释放、打开失败、并发关闭、重插后新句柄；当前mk2的9字节主机缓冲/8字节原生返回按实际字段长度检查 |
| Windows amd64 | 原生C工具链build/test/race/vet；整数HANDLE跨C边界与报文大小 | HID CAPS补零、超长拒绝、input零计数原生约定、权限和设备生命周期 |
| 历史Go源码基线 | 支持该工具链的独立runner测试Go1.13源码；无相应实验标志的项目记录为未运行 | 不由go.mod推出任意旧OS运行支持，也不把CGO=0 stub算原生后端 |
| 报文共同契约 | nil/空、零/非零Report ID、9/61缓冲、短读/写失败/错误保留、重复Close | mk2与mk3分别记录；没有mk3真机时仅有协议/fixture结论 |

## 上游稳定版观察快照

以下在2026年11月9日查询官方release，均非预发布；不代表已并入GID，也不由离线工具自动刷新。

- [HIDAPI 0.15.0](https://github.com/libusb/hidapi/releases/tag/hidapi-0.15.0)，2025-05-18发布。
- [libusb 1.0.30](https://github.com/libusb/libusb/releases/tag/v1.0.30)，2026-05-17发布。
- [sstallion/go-hid 0.15.0](https://github.com/sstallion/go-hid/releases/tag/v0.15.0)，2025-05-23发布；仍是对照候选，不是GID的Go依赖或已选替代品。

固定来源：[dolmen供应快照](https://github.com/dolmen-go/hid/tree/27ac09ac07a3d215b5f360726ecff13b50196fde)、[供应生成器](https://github.com/dolmen-go/hid/blob/27ac09ac07a3d215b5f360726ecff13b50196fde/internal/gen.go)、[历史Linux迁入](https://github.com/b1ug/nb1dev/commit/d2cc7640f362c09c4b21e75b8f6faff0340794fa)、[GID首次迁入](https://github.com/b1ug/gid/commit/19cb4235ec7c9e3d9300a8e4ed6434c810f41394)。
