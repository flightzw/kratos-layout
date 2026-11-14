# Kratos Layout

基于 Kratos v3 的 Go 服务模板，提供 HTTP/gRPC 双协议入口、Protobuf 接口定义、
GORM/MySQL 数据访问、Wire 依赖注入，以及 Todo CRUD 示例。

Todo 使用整数自增 ID，包含名称、创建时间和更新时间。请求参数通过 Protovalidate CEL
注解校验；列表支持页码分页、排序和 AIP 过滤表达式；删除使用 `deleted_at` 软删除。

开发约定见 [开发规范](.agents/rules/development.md)，分层职责见 [AGENTS.md](AGENTS.md)。

## 项目结构

```text
api/
  commons/errno/       公共错误码 proto、枚举和错误辅助函数
  todo/api.proto       OpenAPI 文档元信息
  todo/v1/             Todo 请求、响应和 HTTP/gRPC 生成代码
cmd/server/            服务入口和 Wire 注入代码
configs/               运行配置
internal/
  conf/                配置 proto 和生成代码
  server/              HTTP/gRPC 服务、中间件和路由注册
  service/             请求解析、DTO ↔ DO 转换
  biz/                 领域模型、usecase、仓储接口和业务错误
  data/                GORM 仓储实现、共享连接、DO ↔ PO 转换
    model/             GORM 生成的持久化模型
    query/             GORM 生成的查询代码
pkg/
  aip/                 列表选项、分页和过滤解析
    gorm/              AIP 过滤与排序的 GORM 适配
  apidoc/              Scalar 文档页面
  middleware/validate/ Protovalidate 校验中间件
docs/
  db/db.sql            示例数据库与表结构
  openapi/openapi.json 生成的 OpenAPI 文档
manifest/
  buf/                 API、配置和错误码生成模板
  docker/Dockerfile    容器构建文件
third_party/           本地 proto 依赖
```

请求流向为 `DTO → service → DO → biz → data → PO → MySQL`。
`biz` 声明仓储接口，`data` 实现接口，`cmd/server` 通过 Wire 组装依赖。

## 本地启动

需要 Go（版本以 [go.mod](go.mod) 为准，当前为 `1.26.8`）、MySQL，以及用于开发命令的
Git 和 Make。代码生成还需要访问 Go 模块代理和 Buf 依赖源。

### 1. 初始化数据库

使用具有建库、建表权限的 MySQL 账号执行：

```bash
mysql -u root -p < docs/db/db.sql
```

脚本创建 `demo` 数据库和 `todo` 表。当前数据库构造函数不会执行自动迁移，
即使配置中设置了 `auto_migrate: true`，也需要先准备表结构。

### 2. 配置连接

首次运行时复制环境变量示例；已有 `.env` 时直接编辑原文件：

```bash
cp .env.example .env
```

修改 `.env` 中的连接信息，使数据库名称与建表脚本一致：

```dotenv
KRATOS_DATABASE_SOURCE='app:password@tcp(127.0.0.1:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local'
```

程序自动加载工作目录下的 `.env`，并读取 `KRATOS` 前缀的环境变量。
例如 `KRATOS_DATABASE_SOURCE` 用于解析配置中的 `${DATABASE_SOURCE:...}` 占位符。
`.env` 已被 Git 忽略，真实凭据应通过本地环境或部署配置提供。

[configs/config.yaml](configs/config.yaml) 定义监听地址、超时和数据库配置。
当前运行时使用 MySQL；配置中的 Redis 尚未接入存储客户端。

### 3. 启动服务

在项目根目录执行：

```bash
go run ./cmd/server -conf ./configs
```

| 入口 | 默认地址 |
| --- | --- |
| HTTP | `http://localhost:8000` |
| gRPC | `localhost:9000` |
| Scalar API 文档 | `http://localhost:8000/openapi/reference` |
| OpenAPI JSON | `http://localhost:8000/openapi/spec.json` |

安装开发工具后，可运行 `air` 按 [.air.toml](.air.toml) 配置热重载。

## Todo API

HTTP 与 gRPC 使用同一套 service 实现，均接入恢复、请求日志和 CEL 校验中间件。

| RPC | HTTP | 说明 |
| --- | --- | --- |
| `CreateTodo` | `POST /service/v1/todos` | 创建，JSON 请求体为 `{"name":"待办事项"}` |
| `ListTodo` | `GET /service/v1/todos` | 列表，返回 `data` 和 `total` |
| `GetTodo` | `GET /service/v1/todos/{id}` | 查询，返回 `data` |
| `UpdateTodo` | `PUT /service/v1/todos/{id}` | 更新，JSON 请求体为 `{"name":"新名称"}` |
| `DeleteTodo` | `DELETE /service/v1/todos/{id}` | 软删除 |

创建、更新和删除成功时返回空回复对象；创建回复不包含新记录的 ID，可通过列表查询获取。
更新接口使用完整的名称表单，当前不使用 FieldMask。

```bash
curl -X POST http://localhost:8000/service/v1/todos \
  -H 'Content-Type: application/json' \
  -d '{"name":"整理项目文档"}'

curl -G http://localhost:8000/service/v1/todos \
  --data-urlencode 'page=1' \
  --data-urlencode 'page_size=20' \
  --data-urlencode 'sort=id' \
  --data-urlencode 'order=desc' \
  --data-urlencode 'filter=name : "文档"'
```

列表参数：

| 参数 | 约定 |
| --- | --- |
| `page` | 从 1 开始；0 使用第 1 页 |
| `page_size` | 0 使用默认值 20，最大 100 |
| `sort` | `id`、`name`、`created_at`、`updated_at`；默认 `id` |
| `order` | `asc` 或 `desc`；默认 `asc` |
| `filter` | 可选 AIP 过滤表达式，可引用上述四个字段 |

过滤示例：`name = "示例"`、`id > 10 AND name : "文档"`、
`created_at >= timestamp("2026-01-01T00:00:00Z")`。
SQL 适配能力和边界见 [GORM AIP 适配说明](pkg/aip/gorm/README.md)。

## 开发与代码生成

安装工具，并确保 Go 的可执行文件目录位于 `PATH`：

```bash
make init
export PATH="$(go env GOPATH)/bin:$PATH"
```

`make init` 安装 Wire、Buf、yq、Air、godotenv、genkit 和 golangci-lint。

| 命令 | 作用 |
| --- | --- |
| `make api` | 生成 API Go 代码及 OpenAPI JSON |
| `make errno` | 生成 `api/commons/errno` 的枚举与错误辅助函数 |
| `make config` | 生成 `internal/conf` 配置代码 |
| `make gormgen` | 按 `gorm.gen.yaml` 连接数据库，生成 GORM model/query |
| `make wire` | 生成 `cmd/...` 下的依赖注入代码 |
| `make generate` | 执行 `go generate ./...`，随后整理 Go 模块依赖 |
| `make all` | 依次执行 `api`、`config`、`generate` |
| `make build` | 构建到 `bin/`，注入 Git 版本信息 |
| `make help` | 查看命令列表 |

`make all` 不包含 `make errno` 和 `make gormgen`。修改公共错误码后执行 `make errno`；
修改数据库结构后，先在 [gorm.gen.yaml](gorm.gen.yaml) 中配置对应数据库，再执行 `make gormgen`。
运行时与代码生成器的数据库连接配置相互独立。

请求约束优先在 proto 中使用 CEL 注解，并提供中文错误提示。OpenAPI 的全局 `info`
在 [api/todo/api.proto](api/todo/api.proto) 中配置；字段命名由生成模板的 `naming=proto` 控制。

生成代码只能通过修改源定义或生成配置后重新生成，不能直接编辑，也不能批量整理其 import。

## 验证

```bash
go test ./...
go vet ./...
golangci-lint run
```

service 测试通过仓储替身注入真实 usecase；data 测试使用独立的纯 Go SQLite 数据库，
不依赖本机 MySQL。proto 测试使用 Protovalidate 执行 CEL 规则，覆盖合法值、缺失值和边界。

时间字段测试覆盖 `gtime.Time` 的数据库读写、自动更新时间，以及 nil/零值的响应转换。

## Docker

在项目根目录指定 Dockerfile 构建：

```bash
docker build -f manifest/docker/Dockerfile -t kratos-layout .

docker run --rm -p 8000:8000 -p 9000:9000 \
  -v "$(pwd)/configs:/data/conf:ro" \
  -e KRATOS_DATABASE_SOURCE='app:password@tcp(db-host:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local' \
  kratos-layout
```

将 `db-host`、账号和密码替换为容器能够访问的 MySQL 连接信息，并提前初始化数据库。
Dockerfile 的构建镜像当前为 Go 1.25，需要允许 Go 自动获取 `go.mod` 指定的工具链，
或调整构建镜像以匹配项目 Go 版本。

## 作为新项目模板

1. 修改 `go.mod` 的模块路径，同步更新 Go import 和 proto 的 `go_package`。
2. 检查 `go.mod` 中指向 `../kratos-ext` 的本地 `replace`，按实际依赖调整。
3. 替换 Todo 示例、数据库结构、配置和 OpenAPI 元信息。
4. 运行所需生成命令，执行测试和静态检查后开始开发。

提交采用 Conventional Commits，例如 `feat:`、`fix:`、`refactor:`、`docs:` 和 `test:`。
