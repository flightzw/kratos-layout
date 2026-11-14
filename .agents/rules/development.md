# 项目开发规则

参考 [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)，
结合本项目的 Kratos、Protobuf、Wire 与仓储边界提炼。本文是项目采用的规则集，
不代表全量采用上游约定。分层、资源命名与代码生成遵循 [AGENTS.md](../../AGENTS.md)；
工具检查以 [.golangci.yaml](../../.golangci.yaml) 为准。

“必须”用于正确性和边界约束；“优先”表示默认选择，偏离时在代码或 PR 中说明具体原因。
规则用于新增和修改的手写 Go 代码；历史代码按需修正，生成代码通过生成器更新。

## 命名与可读性

- 使用 `gofmt`，通过 `goimports` 整理导入；导入必须遵循下述四组规则。
- 包名简短、全小写，避免 `util`、`common` 等含义模糊的命名；导入别名用于消除冲突或澄清含义。
- 同一类型的方法使用一致的接收者名。优先提前返回错误，缩小变量作用域，让正常路径保持低缩进。
- 结构体字面量显式写字段名；零值结构体使用 `var`。参数中的布尔值、数字含义不清时，改用具名常量或选项。
- 优先使用零值切片，通过 `len(s) == 0` 判断空集合；API 输出是否为空数组由协议约定决定。

### import 分组

在同一个 `import (...)` 块中，必须按以下顺序分组，组间空一行，组内按导入路径排序；
没有导入项的组直接省略：

1. **标准库**：Go 标准库包。
2. **三方库**：当前模块之外的依赖包。
3. **本地依赖**：导入路径等于 `go.mod` 中的 `module` 路径，或以该路径加 `/` 为前缀的包。
4. **使用 `_` 引用的包**：所有空白导入，无论来自标准库、三方库还是当前模块，统一放在最后一组。

普通导入别名不改变包的分组。通过 `replace` 指向本地目录的外部模块仍属于三方库。
运行 `goimports` 时配置当前模块为本地前缀（`-local`），并在整理后核对四组顺序，
将所有 `_` 导入集中到末组。

```go
import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/flightzw/kratos-layout/internal/biz"
	"github.com/flightzw/kratos-layout/internal/data/query"

	_ "github.com/joho/godotenv/autoload"
	_ "go.uber.org/automaxprocs"
)
```

## 接口与依赖

依据：[接口实现检查](https://github.com/uber-go/guide/blob/master/src/interface-compliance.md)、
[显式初始化](https://github.com/uber-go/guide/blob/master/src/init.md)。

- 接口按值传递，例如 `biz.TodoRepo`；修改实现内部状态时，使用实现类型的指针接收者。
- 在仓储实现旁声明编译期检查，例如 `var _ biz.TodoRepo = (*todoRepo)(nil)`。
- 数据库、日志、时钟等运行依赖通过构造函数传入；测试通过实例注入替身。
  `ProviderSet`、预定义错误等包级声明保留其声明用途，运行时不修改。
- 初始化连接、读取配置等可失败工作放在显式构造函数中并返回错误。
  新增 `init()` 仅用于确定性的注册行为；后台任务由应用生命周期启动。
- 优先使用具名字段组合依赖，避免意外暴露实现方法；服务按 gRPC 契约嵌入
  `Unimplemented<Resource>ServiceServer`。
- 列表查询复用 `biz.ListOption`；只有可选参数确实需要扩展时才引入新的选项模式。
  Wire 注入的必需依赖保持显式构造参数。

## 请求参数校验

- 请求输入约束优先在 proto 中通过 Protovalidate CEL 注解声明，包括必填、长度、格式、
  数值范围、集合约束和跨字段关系。新增或修改规则使用 `(buf.validate.field).cel`
  或 `(buf.validate.message).cel`，规则写法遵循项目的
  [Proto CEL 校验规范](../skills/proto-validate-cel/SKILL.md)。
- 字段值约束使用字段级 CEL；字段存在性和跨字段关系使用消息级 CEL。
  规则使用稳定且非空的 `id`，显式提供清晰的中文错误提示。
  `google.api.field_behavior = REQUIRED` 是接口元数据，不能代替 CEL 校验。
- HTTP 和 gRPC 入口必须在调用 usecase 前执行 Protovalidate 校验。
  调整校验时核实两个入口的运行时接入，不能仅凭 proto 注解已生成就认为校验已生效。
- service 负责 DTO ↔ DO 转换及分页、排序等参数解析；已由 CEL 覆盖的约束不再重复手写。
  只有 CEL 无法表达的输入检查才补充到 service，并说明原因。
- 依赖业务状态、资源存在性、权限或数据库查询的业务规则由 biz 负责。
  CEL 仅描述请求输入约束；新增约束须有接口约定或明确需求依据。

## 错误与日志

依据：[错误包装](https://github.com/uber-go/guide/blob/master/src/error-wrap.md)、
[错误只处理一次](https://github.com/uber-go/guide/blob/master/src/error-once.md)。

- 调用失败后必须返回、恢复或转换错误；明确忽略时说明原因。可能失败的类型断言使用 `value, ok := x.(T)`。
- 需要识别错误时使用标准库 `errors.Is` / `errors.As` 或 Kratos 的错误判定能力，
  不依赖错误文本。标准库与 Kratos 的 `errors` 包同时使用时采用明确别名。
- 内部调用需要补充定位信息时使用简短操作上下文和 `%w`；没有新信息就直接返回。
  仓储跨层返回仍按 AGENTS.md 转为 `biz` 错误，驱动错误类型不能成为上层契约。
- 向上传播的错误由最终处理边界记录，避免每层都记录后再返回；本层已恢复的错误可在本层记录。
  日志使用结构化字段提供操作与资源标识，敏感参数和数据库连接凭据不写入日志。
- 请求校验失败和存储故障通过错误返回；进程退出集中在 `cmd` 的入口，普通业务函数不调用
  `panic`、`log.Fatal` 或 `os.Exit`。

## 并发与资源生命周期

依据：[协程生命周期](https://github.com/uber-go/guide/blob/master/src/goroutine-forget.md)、
[集合所有权](https://github.com/uber-go/guide/blob/master/src/container-copy.md)。

- 每个 goroutine 必须有退出条件或取消信号，并提供等待其结束的方式。
  应用停止时先取消后台任务，再等待退出，最后释放其依赖的客户端。
- 请求链路沿用传入的 `context.Context`，传到 repo 与驱动调用；派生的取消函数由创建方负责调用。
- 锁使用具名值字段，如 `mu sync.Mutex`；含锁的对象通过指针使用，使用后不复制。
  原子状态优先使用标准库 `sync/atomic` 的类型化 API，并统一通过其方法访问。
- channel 默认无缓冲或容量为 1；更大缓冲必须说明容量依据、满载行为与关闭责任。
- 成功获取资源后就近安排 `defer` 清理；循环内需及时释放的资源封装到单次调用中。
  长期客户端由 `Data` 的 cleanup 释放，repo 方法只管理本次操作的资源。
- 保存外部 slice/map 或返回内部集合时，必须明确所有权；可能被调用方修改时复制。
  元素含指针或嵌套集合时，按隔离需求复制内部可变数据，不能把浅拷贝当成完全隔离。

## 类型与性能

依据：[时间类型](https://github.com/uber-go/guide/blob/master/src/time.md)。

- DO 中时间点使用 `time.Time`、时间段使用 `time.Duration`；DTO 和 PO 在各自边界转换。
  必须使用数字表示时间时，在字段名中写明单位；按自然日推进使用日历运算。
- 手写枚举默认让零值表示未指定，业务值从 1 开始；零值是有效默认行为时明确说明。
  proto 枚举遵循协议和兼容性要求，保留已有数值。
- 已知数量时预分配集合容量；简单数字字符串转换优先用 `strconv`；重复转换移出循环。
  引入复杂优化前用 benchmark 或 profile 证明收益。

## 测试与提交验证

依据：[表驱动测试](https://github.com/uber-go/guide/blob/master/src/test-table.md)。

- 相同测试流程的多组输入使用表驱动测试与具名子测试；初始化或断言差异较大的场景独立测试。
- 测试覆盖变更涉及的成功路径、输入边界和失败语义；替身位置遵循 AGENTS.md 的分层测试约定。
  并行测试使用独立状态和资源，通过取消或同步等待协程结束。
- 修改 Go 代码后运行下列检查；涉及共享状态或 goroutine 的变更追加 `go test -race` 检查相关包。
  修改 proto、配置 proto 或 Wire 输入时，先执行对应生成命令，再验证生成结果。

```bash
go test ./...
go vet ./...
golangci-lint run
```

使用 Makefile 安装的 lint 版本和仓库配置。`//nolint:<linter>` 必须指明具体检查并说明理由。
提交说明记录验证结果；因环境或依赖缺失无法完成时，写明未执行的检查与阻碍。

## 提交规范

所有新提交必须遵循 [Conventional Commits 1.0.0](https://www.conventionalcommits.org/zh-hans/v1.0.0/)。
提交前检查暂存区，以实际提交内容确定类型、作用域和描述。

```text
<type>[可选的 scope][!]: <简短描述>

[可选正文]

[可选脚注]
```

- `type` 使用小写，按主要变更目的选择：

  | 类型 | 用途 |
  | --- | --- |
  | `feat` | 新增功能 |
  | `fix` | 修复缺陷 |
  | `refactor` | 重构，未新增功能或修复缺陷 |
  | `perf` | 性能优化 |
  | `docs` | 文档变更 |
  | `test` | 测试变更 |
  | `style` | 格式调整，不改变代码行为 |
  | `build` | 构建或依赖调整 |
  | `ci` | 持续集成配置 |
  | `chore` | 其他维护工作 |
  | `revert` | 回退提交 |

- `scope` 可选，使用括号标识受影响模块，例如 `todo`、`data`、`api`、`deps`。
  依赖维护也可沿用项目的 `chore(deps)`。
- 冒号后必须有一个空格，描述必须非空，准确概括结果；中文、英文均可，内容尽量精炼简短。
  正文和脚注按需补充，与前一部分空一行。
- 不兼容变更必须在冒号前标记 `!`，或使用 `BREAKING CHANGE: <说明>` 脚注。
  根据需要说明兼容性影响和迁移方式。
- 一个提交应围绕一个明确目的；可独立交付的无关变更优先分开提交。
  用户明确要求整体提交时，按整体主要目的命名，并标记其中的不兼容变更。

示例：`feat(todo): 支持名称过滤`、`fix(data): 修复分页查询取消失效`、
`docs: 更新本地启动说明`、`refactor(api)!: 调整 Todo 请求与响应结构`。

## 项目适配说明

- 原子操作采用标准库类型化 API；不为编码风格新增 `go.uber.org/atomic` 依赖。
- lint 使用现有 golangci-lint 配置；不额外要求上游列出的独立 `golint` 工具。
- 非导出包级变量沿用项目命名，不统一增加下划线前缀。
- Go 版本以 go.mod 为准；循环变量捕获按当前语言语义判断，不机械添加 `tt := tt`。
