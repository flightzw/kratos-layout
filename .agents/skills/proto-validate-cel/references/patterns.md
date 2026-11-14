# CEL 注解模式

以下阈值、字段和值域仅是示例，按实际接口约定替换。片段均需所在文件导入 `buf/validate/validate.proto`。

## 单字段与多个错误

一个字段多个独立规则可重复写 `(buf.validate.field).cel = {...}`。需要首个错误有确定优先级时，将条件写在同一表达式内：

```proto
message NameInput {
  string name = 1 [(buf.validate.field).cel = {
    id: "name_input.name.valid"
    expression:
      "this == '' ? '名称不得为空' : "
      "size(this) > 100 ? '名称最多允许 100 个字符' : ''"
  }];
}
```

布尔返回式也只使用 CEL，适合固定提示：

```proto
message GetInput {
  int64 id = 1 [(buf.validate.field).cel = {
    id: "get_input.id.positive"
    message: "id 必须大于 0"
    expression: "this > 0"
  }];
}
```

表达式速查（都可包成 `条件 ? '' : '错误提示'`）：

| 意图 | CEL 条件 | 语义 |
| --- | --- | --- |
| 分页大小 | `this >= 0 && this <= 100` | 0 可表示服务端默认值，仅当接口这样约定 |
| 无符号 ID | `this > 0u` | uint32/uint64 |
| 升降序 | `this in ['', 'asc', 'desc']` | 空串为默认值，仅当接口允许 |
| 排序字段 | `this in ['', 'id', 'name']` | 必须使用真实可排序字段 |
| 选填邮箱 | `this == '' || this.isEmail()` | 需确认运行时支持该扩展 |
| ASCII 编号 | `this.matches('^[A-Z][A-Z0-9_-]*$')` | 整串格式匹配 |
| 枚举白名单 | `this in [1, 2]` | 按实际 enum 定义替换；同时排除未知值 |

## 必填与选填

`optional` 标量的字段 CEL 检查已提供的值；父消息 CEL 检查是否提供。两者分别处理缺失和空串：

```proto
message OptionalNameInput {
  option (buf.validate.message).cel = {
    id: "optional_name_input.name.present"
    expression: "has(this.name) ? '' : '必须提供 name 字段'"
  };
  optional string name = 1 [(buf.validate.field).cel = {
    id: "optional_name_input.name.non_empty"
    expression: "this != '' ? '' : 'name 不得为空'"
  }];
}
```

父消息缺失时，子消息全部规则均不会执行；检查必须放在仍会执行的父级：

```proto
message FormInput {
  option (buf.validate.message).cel = {
    id: "form_input.form.present"
    expression: "has(this.form) ? '' : 'form 不得缺失'"
  };
  message Form {
    string name = 1 [(buf.validate.field).cel = {
      id: "form.name.non_empty"
      expression: "this != '' ? '' : '名称不得为空'"
    }];
  }
  Form form = 1;
}
```

## 跨字段与 oneof

可选范围同时存在时才比较：

```proto
message RangeInput {
  option (buf.validate.message).cel = {
    id: "range_input.bounds.ordered"
    expression:
      "!has(this.start) || !has(this.end) || this.start <= this.end"
      " ? '' : 'start 不得大于 end'"
  };
  optional int64 start = 1;
  optional int64 end = 2;
}
```

原生 oneof 已保证至多一个成员，消息级 CEL 保证至少一个；成员已设置为空串仍算存在，需要额外值规则：

```proto
message TargetInput {
  option (buf.validate.message).cel = {
    id: "target_input.target.present"
    expression: "has(this.id) || has(this.name) ? '' : '必须提供 id 或 name'"
  };
  oneof target {
    int64 id = 1 [(buf.validate.field).cel = {
      id: "target_input.id.positive"
      expression: "this > 0 ? '' : 'id 必须大于 0'"
    }];
    string name = 2 [(buf.validate.field).cel = {
      id: "target_input.name.non_empty"
      expression: "this != '' ? '' : 'name 不得为空'"
    }];
  }
}
```

## repeated 与 map

只在集合字段本身挂 CEL，使用宏校验元素；不要通过 `.repeated.items`、`.map.keys` 或 `.map.values` 配置规则。

```proto
message CollectionInput {
  repeated int64 ids = 1 [(buf.validate.field).cel = {
    id: "collection_input.ids.valid"
    expression:
      "size(this) == 0 ? 'ids 至少包含一个元素' : "
      "!this.all(x, x > 0) ? 'ids 中每个 id 必须大于 0' : ''"
  }];
  map<string, string> labels = 2 [(buf.validate.field).cel = {
    id: "collection_input.labels.valid"
    expression:
      "size(this) > 20 ? 'labels 最多允许 20 项' : "
      "!this.all(k, k != '' && this[k] != '') ? '标签键和值不得为空' : ''"
  }];
}
```

这些集合宏的错误定位通常落在整个集合字段；如需要逐元素路径，验证运行时支持的方案再实现，不承诺宏自动生成索引路径。

## 部分更新与 FieldMask

创建规则不能直接套用到 PATCH 的共享资源消息，否则未更新字段也可能被校验。先读取 service 的 `fieldmask.Update` 约定，确认空 mask、`*`、嵌套路径及字段清空语义。优先把与更新掩码相关的约束放在 Update 请求消息上，避免改变共用资源消息的语义。

以下示例**仅接受显式的顶层 `name` 路径，且空 mask 非法**；该约定不是当前所有接口的默认值。文件还需导入 `google/protobuf/field_mask.proto`：

```proto
message PatchInput {
  option (buf.validate.message).cel = {
    id: "patch_input.update_mask.valid"
    expression:
      "!has(this.update_mask) ? 'update_mask 不得缺失' : "
      "size(this.update_mask.paths) == 0 ? 'update_mask 不得为空' : "
      "!this.update_mask.paths.all(p, p == 'name') ? '仅允许更新 name' : ''"
  };
  option (buf.validate.message).cel = {
    id: "patch_input.name.non_empty_when_updated"
    expression:
      "!has(this.update_mask) ? '' : "
      "!('name' in this.update_mask.paths) || this.name != ''"
      " ? '' : '更新 name 时名称不得为空'"
  };
  string name = 1;
  google.protobuf.FieldMask update_mask = 2;
}
```

不要用 `this.name != ''` 代替“本次更新选中了 name”，否则会混淆字段清空和字段未更新。
