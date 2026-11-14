---
name: proto-validate-cel
description: 为当前 Kratos 项目的 proto 请求消息编写、修改或审查 Protovalidate CEL 校验注解，提供自定义中文错误提示；适用于参数必填、格式、范围、集合和跨字段约束。
---

# Proto CEL 校验注解

在 `api/` 的 proto 请求消息上表达输入约束。所有新增或改写的 Protovalidate 校验规则只使用 `(buf.validate.field).cel` 和 `(buf.validate.message).cel`，错误提示由规则显式控制。

## 工作流程

1. 阅读项目 `AGENTS.md`、目标 proto 及调用它的 service；检查 `buf.yaml`、`buf.lock` 和生成命令。以锁定依赖的能力为准，上游 `main` 仅作参考。
2. 按请求区分创建、查询、更新的语义，确定每个目标字段的合法值、空值、缺失值及边界。不擅自引入名称长度上限、分页上限或枚举值等产品约束；无法推断时只询问影响实现的缺失条件。
3. 按下文选择字段级或消息级 CEL。常用表达式、集合及部分更新参考 [references/patterns.md](references/patterns.md)，仅阅读当前任务相关部分。
4. 检查每条规则的通过、失败、缺失、零值和边界行为；按“验证与交付”完成检查。

## 注解与错误提示

- 导入 `buf/validate/validate.proto`；保留原有包名、字段编号、HTTP 映射和其他 API 元数据。
- 仅使用完整 `.cel` 规则。不要引入 `.cel_expression` 简写、内建 `.string` / `.int32` / `.repeated` / `.map` 等规则、`.required`、`.ignore`、`.oneof` 规则或 PGV 的 `(validate.rules)`。必填和条件跳过都用 CEL 表达。
- `google.api.field_behavior = REQUIRED` 属于 API 元数据，可以按接口约定保留；它不能代替具有自定义错误提示的 CEL 规则。
- 单字段值约束放在字段上，`this` 就是字段值。跨字段关系、字段存在性放在包含这些字段的消息上，`this` 是该消息。
- 每条规则使用非空、稳定的 `id`，在所属字段或消息作用域内唯一。新增规则建议 `<request>.<field>.<constraint>`，如 `create_todo.name.non_empty`；保留现有规则的稳定标识。
- 默认沿用项目现有的**字符串返回式**：合法时返回 `''`，非法时返回中文错误字符串，省略 `message`。
- 固定提示也可采用**布尔返回式**：表达式返回布尔值，显式设置非空 `message`，`true` 通过、`false` 失败。
- 同一规则只选择一种返回式。字符串返回式不要同时设置 `message`，避免不同版本对提示优先级的差异。
- 提示明确指出字段和修正条件，例如“名称不得为空”“page_size 必须在 0 到 100 之间”。沿用用户指定语言；避免把令牌、密码等输入拼入错误提示。
- 一个规则负责一个可解释约束；多个规则不要依赖执行顺序。若同一字段要求只返回一个有优先级的提示，在单个字符串表达式中使用条件分支。

```proto
message CreateTodoRequest {
  string name = 2 [(buf.validate.field).cel = {
    id: "create_todo.name.non_empty"
    expression: "this != '' ? '' : '名称不得为空'"
  }];
}
```

以上只拒绝空字符串，不隐含 trim；只有约定要求拒绝纯空白时才增加对应规则。

## 存在性与类型

- proto3 普通标量缺失后呈现零值；仅凭标量值无法区分“未传”和“显式传零值”。确需区分时先明确接口是否允许改为 `optional`，不要为添加注解擅改字段存在性。
- `optional`、消息字段、oneof 成员缺失时，其字段级规则默认被跳过。必填检查写在父消息级：`has(this.form)`，而不是在 `form` 字段的 CEL 中判空。
- 访问可缺失的嵌套字段时用 `has` 保护。必填用 `has(this.form) && ...`，选填用 `!has(this.form) || ...`；也可用三元表达式先处理缺失分支。
- `has` 的参数是字段选择，例如 `has(this.name)`；不要写 `has(this)`。对隐式存在性的标量，`has` 不能区分显式传零值与缺失。
- 整数 CEL 类型要匹配：有符号整数用 `0`，无符号整数用 `0u`；枚举使用实际数值集合，包含/排除零值由接口语义决定。
- `all` 对空集合返回真；要求非空时另加 `size(this) > 0`。字符串 `size` 按 Unicode 字符计数，bytes 按字节；错误提示中的长度单位应一致。
- 正则有 proto 字符串和 CEL 字符串两层转义；优先使用 `[0-9]` 等减少转义，整串匹配加 `^`、`$`。
- 使用 `isEmail()` 等扩展前确认锁定版本支持；不要从其他语言搬入 `trim()`、`len()`、`now()` 等未确认函数。运行时求值异常不是可控的校验提示。

## 项目边界

注解只表达 DTO 输入约束。数据库唯一性、资源存在性、权限、状态迁移等规则留在 biz，保持 service DTO ↔ DO、biz 仅 DO、data DO ↔ PO 的分层。

运行时是否执行 CEL 需要单独核实：检查 HTTP 和 gRPC 的 middleware 或 service 边界是否调用 Protovalidate。`fieldbehavior.ValidateRequiredFields` 只检查 field behavior；生成的注解描述符也不是校验器。只编写注解时报告接入缺口；用户要求接入时再实现，并使用项目的 biz 类型错误约定。自定义 `message`/返回字符串控制的是 violation 提示，最终 HTTP/gRPC 错误外形取决于运行时适配。

## 验证与交付

- proto 修改后按项目配置运行 `buf format`、`buf build`、`buf lint`，再用 `make api` 生成；不手改 `*.pb.go`、`*_grpc.pb.go`、`*_http.pb.go`。区分原有 lint 问题和本次引入的问题。
- `buf build` 只能证明 schema 可编译。用兼容锁定 schema 的 Protovalidate 运行时编译实际规则，并执行合法、非法、边界、缺失及零值样例；检查 violation 的规则 ID 和自定义提示。项目已有校验测试时优先扩展它；若仅在临时目录验证也要说明范围。
- 对 optional、父消息必填、oneof、更新掩码，必须覆盖“字段缺失”以及“显式传零值”分支。涉及 HTTP/gRPC 接入时分别验证两个入口。
- 交付说明修改位置、约束、验证结果及运行时接入状态。工具、依赖或网络不可用时准确报告未验证项，不把 schema 编译称为 CEL 执行通过。

## 权威参考

- [validate.proto](https://github.com/bufbuild/protovalidate/blob/main/proto/protovalidate/buf/validate/validate.proto)：`Rule`、`FieldRules.cel`、`MessageRules.cel` 和默认存在性行为。
- [Custom CEL rules](https://protovalidate.com/schemas/custom-rules/)：字段级、消息级 CEL 的使用方式。

本技能的 CEL-only 和默认中文提示是本项目约定，不是上游限制。
