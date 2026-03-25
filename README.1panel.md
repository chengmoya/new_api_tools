# `new_api_tools` 1Panel 部署说明

本文档用于在 1Panel 中直接部署 [`new_api_tools`](new_api_tools:1)。目标是：**使用本地源码构建、尽量少改参数、直接构建并启动**。

## 第一步：上传项目源码到 1Panel 项目目录

将完整的 [`new_api_tools`](new_api_tools:1) 目录上传到 1Panel 的项目目录中。

确认目录内至少包含以下文件：

- [`Dockerfile`](new_api_tools/Dockerfile)
- [`docker-compose.1panel.yml`](new_api_tools/docker-compose.1panel.yml)

> 重点：必须上传源码本体，而不是只上传编排文件，因为本方案使用 [`build`](new_api_tools/docker-compose.1panel.yml:12) 方式从本地源码构建镜像。

## 第二步：在编排中粘贴 `docker-compose.1panel.yml` 内容

在 1Panel 新建 Compose / 编排项目时，使用 [`docker-compose.1panel.yml`](new_api_tools/docker-compose.1panel.yml) 的内容。

如果你已经把该文件一并上传，也可以直接选择这个文件作为编排文件。

## 第三步：只修改最少变量

打开 [`docker-compose.1panel.yml`](NEWAPI-开发/new_api_tools/docker-compose.1panel.yml)，优先确认以下配置。

### 1）必须修改：`SQL_DSN`

[`SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:26) 是业务库连接串，用于读取 NewAPI 主业务数据。

MySQL 示例：

```yaml
SQL_DSN: newapi_user:your_business_db_password@tcp(mysql:3306)/newapi?charset=utf8mb4&parseTime=True&loc=Local
```

PostgreSQL 示例：

```yaml
SQL_DSN: postgres://newapi_user:your_business_db_password@postgres:5432/newapi?sslmode=disable
```

### 2）建议新增：`TOOLS_SQL_DSN`

[`TOOLS_SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:27) 是 tools 配置库连接串，用于存放工具侧配置。未设置时，程序会自动回退到 [`SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:26)；若 tools 库连接失败，也会回退到兼容缓存模式，保证旧配置仍可读取，见 [`ToolsDSN()`](NEWAPI-开发/new_api_tools/backend/internal/config/config.go:111) 与 [`NewConfigStore()`](NEWAPI-开发/new_api_tools/backend/internal/storage/config_store.go:49)。

MySQL 示例：

```yaml
TOOLS_SQL_DSN: newapitools_user:your_tools_db_password@tcp(mysql:3306)/newapitools?charset=utf8mb4&parseTime=True&loc=Local
```

PostgreSQL 示例：

```yaml
TOOLS_SQL_DSN: postgres://newapitools_user:your_tools_db_password@postgres:5432/newapitools?sslmode=disable
```

### 3）必须修改：`ADMIN_PASSWORD`

设置后台管理员密码，例如：

```yaml
ADMIN_PASSWORD: MyStrongPassword123!
```

### 4）可选修改：`REDIS_CONN_STRING`

[`REDIS_CONN_STRING`](NEWAPI-开发/new_api_tools/.env.example:89) 为 Redis 连接串。已配置时用于缓存恢复与查询加速；未配置时可继续运行，但缓存能力会退化。

示例：

```yaml
REDIS_CONN_STRING: redis://:your_redis_password@redis:6379/0
```

### 5）可直接复制的 `.env` 片段

```env
SQL_DSN=newapi_user:your_business_db_password@tcp(mysql:3306)/newapi?charset=utf8mb4&parseTime=True&loc=Local
TOOLS_SQL_DSN=newapitools_user:your_tools_db_password@tcp(mysql:3306)/newapitools?charset=utf8mb4&parseTime=True&loc=Local
ADMIN_PASSWORD=MyStrongPassword123!
REDIS_CONN_STRING=redis://:your_redis_password@redis:6379/0
JWT_SECRET_KEY=replace_with_a_fixed_random_secret
JWT_EXPIRE_HOURS=24
TIMEZONE=Asia/Shanghai
LOG_LEVEL=info
```

### 6）本次配置落地范围

当前已落到 tools 配置库存储的配置项，主要包括：

- AI 监控配置
- 自动分组配置
- 模型监控相关配置（如自定义分组）

这些配置统一通过 [`ConfigStore`](NEWAPI-开发/new_api_tools/backend/internal/storage/config_store.go:41) 读写；当配置项在 tools 库不存在时，会先尝试读取旧缓存并在首次读取时回填到 [`tool_configs`](NEWAPI-开发/new_api_tools/backend/internal/storage/config_store.go:131)。

### 7）其他变量可保持默认

以下变量一般可以不改：

- [`JWT_SECRET_KEY`](NEWAPI-开发/new_api_tools/docker-compose.1panel.yml:31)
- [`JWT_EXPIRE_HOURS`](NEWAPI-开发/new_api_tools/docker-compose.1panel.yml:34)
- [`TIMEZONE`](NEWAPI-开发/new_api_tools/docker-compose.1panel.yml:37)
- [`LOG_LEVEL`](NEWAPI-开发/new_api_tools/docker-compose.1panel.yml:40)

## 第四步：点击构建并启动

在 1Panel 中执行：

1. 构建
2. 启动

本编排会使用 [`context: .`](new_api_tools/docker-compose.1panel.yml:13) 和 [`dockerfile: Dockerfile`](new_api_tools/docker-compose.1panel.yml:14) 从当前源码目录直接构建，不依赖远程业务镜像。

默认对外端口为 [`1145:80`](new_api_tools/docker-compose.1panel.yml:17)。

## 第五步：访问地址与登录方式

启动成功后，浏览器访问：

```text
http://服务器IP:1145
```

登录方式：

- 用户名：管理员账号（如系统默认登录页所示）
- 密码：你在 [`ADMIN_PASSWORD`](new_api_tools/docker-compose.1panel.yml:25) 中设置的值

如果你配置了域名和反向代理，也可以直接通过对应域名访问。

## FAQ

### 1. 不填 `JWT_SECRET_KEY` 会怎样？

可以正常启动。

但如果 [`JWT_SECRET_KEY`](new_api_tools/docker-compose.1panel.yml:31) 留空，程序通常会在启动时自动生成临时密钥。这样做的影响是：**容器重启后旧 token 可能失效，需要重新登录**。

如果你希望登录态在重启后依然稳定，建议手动设置固定的 [`JWT_SECRET_KEY`](new_api_tools/docker-compose.1panel.yml:31)。

### 2. 数据库 / Redis 连不上，怎么排查？

优先检查这几项：

1. [`SQL_DSN`](new_api_tools/docker-compose.1panel.yml:22) 是否填写正确
2. [`REDIS_CONN_STRING`](new_api_tools/docker-compose.1panel.yml:28) 是否填写正确
3. 1Panel 中数据库 / Redis 的服务名、端口、用户名、密码是否正确
4. 当前 Compose 项目是否能访问对应数据库 / Redis
5. 数据库是否已经创建目标库，例如 `newapi`

常见错误包括：

- 主机名写错，例如把服务名写成了不存在的地址
- 端口写错，例如 MySQL 不是 `3306` / PostgreSQL 不是 `5432`
- 密码中包含特殊字符但未正确处理
- 数据库本身未启动或未放通访问

### 3. 如何升级？

升级方式尽量保持简单：

1. 用新版本源码覆盖旧的 [`new_api_tools`](NEWAPI-开发/new_api_tools/README.1panel.md) 目录
2. 保留原有 [`docker-compose.1panel.yml`](NEWAPI-开发/new_api_tools/docker-compose.1panel.yml) 配置
3. 在 1Panel 中重新执行“构建”
4. 再执行“启动”或“重建”

因为本方案是本地源码构建，所以**重新构建**就是升级的核心动作。

### 4. 双数据库升级 / 迁移建议

从旧本地配置迁移到 tools DB 时，建议按以下顺序执行：

1. 先保留原有 [`SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:26)，确保业务读库不变
2. 新增 [`TOOLS_SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:27)，指向独立的 tools 配置库
3. 重启服务后，让系统在首次读取相关配置时自动迁移旧值
4. 确认常用页面配置已成功读取并保存，再决定是否清理旧缓存

兼容策略与当前实现一致：

- 优先读取 tools 库中的 [`tool_configs`](NEWAPI-开发/new_api_tools/backend/internal/storage/config_store.go:131)
- 若 tools 库中不存在对应配置，则回退读取旧缓存
- 首次读取到旧缓存后，会自动惰性回填到 tools 库，见 [`GetJSON()`](NEWAPI-开发/new_api_tools/backend/internal/storage/config_store.go:210) 与 [`GetValue()`](NEWAPI-开发/new_api_tools/backend/internal/storage/config_store.go:239)
- 若 [`TOOLS_SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:27) 未配置，程序默认复用 [`SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:26)
- 若 tools 库不可用，配置存储会回退到兼容缓存模式，避免升级后立即中断

### 5. 如何回滚？

推荐两种简单做法：

1. 保留一份旧版本源码目录，需要回滚时切回旧源码重新构建
2. 保留旧的编排副本或旧镜像 tag，需要回滚时重新使用旧版本配置启动

如果你的发布流程较规范，建议每次升级前都备份一份旧版 [`docker-compose.1panel.yml`](new_api_tools/docker-compose.1panel.yml) 和对应源码。

### 6. 常见排障

#### MySQL `Access denied`

优先检查 [`SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:26) / [`TOOLS_SQL_DSN`](NEWAPI-开发/new_api_tools/.env.example:27) 中的用户名、密码、库名是否正确；同时确认数据库账号已授权当前来源地址访问目标库。

#### 容器内 DNS 解析失败

如果在 1Panel 中使用服务名连接 MySQL、PostgreSQL 或 Redis，需确认当前编排与目标服务位于同一 Docker 网络；服务名应使用 1Panel 实际容器服务名，不要填写宿主机面板展示名。

#### Redis 连接失败

优先检查 [`REDIS_CONN_STRING`](NEWAPI-开发/new_api_tools/.env.example:89) 的协议、密码、端口和 DB 编号；若密码包含特殊字符，建议先进行 URL 编码后再写入连接串。

## 编排特点说明

[`docker-compose.1panel.yml`](new_api_tools/docker-compose.1panel.yml) 已按 1Panel 场景做了简化：

- 仅保留一个服务：[`newapi-tools`](new_api_tools/docker-compose.1panel.yml:11)
- 使用本地源码构建，不拉业务远程镜像
- 不内置 MySQL、PostgreSQL、Redis 服务
- 支持直接复用 1Panel 自带数据库与 Redis
- 数据目录使用相对路径挂载：[`./data:/app/data`](new_api_tools/docker-compose.1panel.yml:44)
- 已内置健康检查：[`/api/health`](new_api_tools/docker-compose.1panel.yml:47)
