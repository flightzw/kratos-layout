# GORM AIP 适配

参考 [aip-go/ents](https://github.com/go-kratos/aip-go/tree/main/ents)，将
`go.einride.tech/aip/filtering.Filter` 和 `ordering.OrderBy` 转换成 GORM scope。

```go
import (
    "go.einride.tech/aip/filtering"
    "go.einride.tech/aip/ordering"

    aipgorm "github.com/flightzw/kratos-layout/pkg/aip/gorm"
)

declarations, err := filtering.NewDeclarations(
    filtering.DeclareStandardFunctions(),
    filtering.DeclareIdent("display_name", filtering.TypeString),
    filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
)
if err != nil {
    return err
}
filter, err := filtering.ParseFilterString(`display_name : "demo"`, declarations)
if err != nil {
    return err
}
var order ordering.OrderBy
if err := order.UnmarshalString("created_at desc"); err != nil {
    return err
}
if err := order.ValidateForPaths("display_name", "created_at"); err != nil {
    return err
}
fields := aipgorm.FieldMap{"display_name": "name"}
err = db.WithContext(ctx).Model(&Todo{}).Scopes(
    aipgorm.ApplyFilter(filter, aipgorm.WithFilterFieldMap(fields)),
    aipgorm.ApplyOrderBy(order, aipgorm.WithOrderByFieldMap(fields)),
).Find(&todos).Error
```

- 支持 `AND`、`OR`、`NOT`、隐式 AND（`FUZZY`）、六种比较运算、布尔字段和布尔常量节点。
  隐式 AND 需在解析阶段声明 `FUZZY` 的布尔函数重载；当前 AIP 解析器将
  `true`/`false` 文本解析为标识符，不会自动生成布尔常量。
- 比较支持字段与值、字段与字段、反向比较及常量比较。值通过参数绑定传入 SQL。
- `:` 支持字符串子串匹配，按字面量转义 `%`、`_`，不支持集合、JSON 或时间戳的 has 运算。
- 支持字符串、整数、无符号整数、浮点数、布尔值、时间戳与时长常量节点，
  以及 `timestamp("RFC3339")`、`duration("1h")`；时长以 `time.Duration`（纳秒）绑定。
  时间戳字段与字符串比较时，将字符串转换为 `time.Time`。
- 排序保留字段顺序和方向，追加到已有排序之后。分页和唯一键排序由调用方处理。
- 字段路径直接表示数据库列；`table.column` 表示限定列名，不自动生成 JOIN 或 JSON 查询。
  字段映射在创建选项时复制，未映射字段保留原名。映射不是白名单；调用方应通过
  filtering declarations 和 `ValidateForPaths` 限定公开字段。
- 空过滤和空排序不修改查询。非法列路径、无法转换的表达式会设置 `DB.Error`，
  阻止查询执行；调用方须检查最终查询的错误。不支持的函数和表达式不会被忽略。

本包仅用于 data 层，返回的错误由仓储转换为业务错误。
