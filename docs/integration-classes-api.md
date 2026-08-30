# 班级名单开放接口

本文档描述 ClassOrbit 提供给 TypeMatch 或其他受信任系统的班级名单只读接口。

## 接口概览

| 项目 | 内容 |
| --- | --- |
| 方法 | `GET` |
| 路径 | `/api/integration/classes` |
| 鉴权 | Bearer Token |
| 请求格式 | 无请求体 |
| 响应格式 | `application/json; charset=utf-8` |
| 缓存策略 | `Cache-Control: no-store` |

接口返回当前教师账号下的全部班级及学生名单，不返回积分、考勤、教师密码或教师登录会话。正常数据中的学生 `id` 来自学号，而不是数据库内部 ID。每次请求返回完整名单快照，目前不支持分页或增量同步。

## 服务端配置

在 ClassOrbit 的运行环境中设置共享密钥：

```dotenv
CLASS_SYSTEM_TOKEN=请替换为足够长的随机密钥
```

使用 Docker Compose 时可写入项目的 `.env` 文件，然后重新创建容器使配置生效：

```bash
docker compose up -d --force-recreate
```

建议使用密码管理器或随机数工具生成至少 32 字节的密钥。调用方必须配置完全相同的值。不要把共享密钥放在 URL、查询参数、前端 JavaScript 或日志中。

## 请求

### 查询参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `teacher_username` | string | 是 | ClassOrbit 中已初始化的教师账号用户名；比较时不区分大小写。 |

### 请求头

| 请求头 | 必填 | 说明 |
| --- | --- | --- |
| `Authorization` | 是 | 格式必须为 `Bearer <CLASS_SYSTEM_TOKEN>`。 |
| `Accept` | 否 | 推荐使用 `application/json`。 |
| `X-Teacher-Username` | 否 | 调用方需要同时传递教师标识时使用；一旦提供，必须与查询参数中的用户名一致，比较时不区分大小写。 |

接口不使用教师后台的 Cookie，也不接受教师账号密码。

### 原始 HTTP 示例

```http
GET /api/integration/classes?teacher_username=teacher001 HTTP/1.1
Host: classorbit.example.com
Accept: application/json
Authorization: Bearer your-shared-secret
X-Teacher-Username: teacher001
```

### curl 示例

```bash
curl --fail-with-body -G "https://classorbit.example.com/api/integration/classes" \
  --data-urlencode "teacher_username=teacher001" \
  -H "Accept: application/json" \
  -H "X-Teacher-Username: teacher001" \
  -H "Authorization: Bearer your-shared-secret"
```

## 成功响应

状态码：`200 OK`

```json
{
  "classes": [
    {
      "name": "三 1 班",
      "students": [
        {
          "id": "2026001",
          "name": "张三"
        },
        {
          "id": "2026002",
          "name": "李四"
        }
      ]
    },
    {
      "name": "四 2 班",
      "students": []
    }
  ]
}
```

### 响应字段

| JSON 路径 | 类型 | 是否总是存在 | 说明 |
| --- | --- | --- | --- |
| `classes` | array | 是 | 班级数组；没有班级时为 `[]`，不会返回 `null`。 |
| `classes[].name` | string | 是 | 班级显示名称，例如 `三 1 班`。 |
| `classes[].students` | array | 是 | 该班学生数组；空班级返回 `[]`。 |
| `classes[].students[].id` | string | 是 | 学生学号。必须按字符串处理，以保留 `001` 之类的前导零。若历史异常数据没有学号，服务端会回退为数据库内部 ID 的十进制字符串。 |
| `classes[].students[].name` | string | 是 | 学生姓名。 |

### 排序规则

- 班级按 ClassOrbit 中的创建顺序排列。
- 学生优先按学号的数值顺序排列，再按学号文本和内部创建顺序排列。
- 调用方不应依赖数组位置作为标识；学生应使用 `id`，班级应使用当前的 `name` 进行匹配。教师修改年级或班号后，班级名称也会变化。

### TypeScript 数据类型

```ts
export type IntegrationClassesResponse = {
  classes: Array<{
    name: string
    students: Array<{
      id: string
      name: string
    }>
  }>
}
```

## 错误响应

所有业务错误使用统一结构：

```json
{
  "error": "错误说明"
}
```

| HTTP 状态码 | 场景 | 典型错误信息 | 调用方处理建议 |
| --- | --- | --- | --- |
| `400 Bad Request` | 缺少 `teacher_username` | `缺少教师用户名` | 修正请求，不要自动重试。 |
| `400 Bad Request` | `X-Teacher-Username` 与查询参数不一致 | `教师用户名不一致` | 统一两个用户名后重试。 |
| `401 Unauthorized` | 缺少 `Authorization` | `缺少 Authorization 凭证` | 添加 Bearer Token。 |
| `401 Unauthorized` | Authorization 格式不是 `Bearer <token>` | `Authorization 格式错误` | 修正请求头格式。 |
| `403 Forbidden` | 共享密钥错误，或服务端未配置共享密钥 | `共享密钥错误` | 核对双方配置；不要持续重试。 |
| `404 Not Found` | 教师账号尚未初始化或用户名不存在 | `教师账号不存在` | 核对账号，并确认 ClassOrbit 已完成首次初始化。 |
| `405 Method Not Allowed` | 使用了 GET 以外的方法 | 由 HTTP 路由返回 | 改用 GET。 |
| `500 Internal Server Error` | 数据库或服务端异常 | `操作失败，请稍后重试` | 使用退避策略有限重试，并通知运维。 |

## 接入约定与安全要求

1. 生产环境必须使用 HTTPS，避免共享密钥和学生名单被窃听。
2. 共享密钥代表读取全部班级名单的权限，应只保存在服务端环境变量或密钥管理系统中。
3. 接口返回学生个人信息，调用方应限制日志内容、缓存范围和数据保留时间。
4. 响应明确禁止缓存；中间代理和调用方也应遵守 `Cache-Control: no-store`。
5. `id` 始终按字符串解析，不要转换为数字。
6. 当前系统只支持一个教师账号；请求用户名用于确认调用方读取的是目标教师的数据。
7. 共享密钥轮换后，需要更新调用方配置并重启或重新创建 ClassOrbit 服务。
8. 若前置网关返回 `429`，或接口返回 `5xx`，可采用指数退避；对其他 `4xx` 应先修正配置或请求，不应无休止重试。

## 联通性检查

先确认服务健康：

```bash
curl --fail "https://classorbit.example.com/api/health"
```

健康响应为：

```json
{"ok":true}
```

再执行前述名单请求。收到 `200` 且 `classes` 为数组，即表示鉴权和数据格式均正常。
