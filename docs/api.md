# ClassOrbit HTTP API

本文是前后端接口的快速索引。服务前缀为 `/api`，所有 JSON 请求使用
`Content-Type: application/json`，所有 JSON 响应使用
`application/json; charset=utf-8`。浏览器端通过 HttpOnly Cookie
`classorbit_session` 维持教师会话；不需要手动在请求中拼接 Cookie。

## 通用约定

- 成功响应直接返回资源，删除/恢复等无资源操作返回 `{ "ok": true }`。
- 失败响应统一为 `{ "error": "错误说明" }`。常见状态码为 `400`（参数错误）、
  `401`（未登录）、`404`（资源不存在）、`409`（状态冲突）和 `500`（服务端错误）。
- `POST`、`PATCH`、`PUT` 请求体限制为 1 MiB；文件上传接口有各自的大小限制。
- 教师接口必须先登录。公开接口只读公开课堂数据；集成接口使用独立 Bearer Token。
- 删除班级、学生和考勤默认是软删除；考勤永久删除和数据库恢复属于高风险操作。
- 时间以服务器本地时区保存，日期使用 `YYYY-MM-DD`，课时使用 `HH:mm`。

## 系统与认证

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/health` | 公开 | 返回 `ok`、数据库状态、版本和构建提交。 |
| `GET` | `/api/auth` | 公开 | 返回 `{ initialized, authenticated, username }`。 |
| `POST` | `/api/setup` | 未初始化 | 创建首个教师账号并建立会话；重复初始化返回 `409`。 |
| `POST` | `/api/auth` | 公开 | 使用 `{ username, password }` 登录。 |
| `DELETE` | `/api/auth` | 公开 | 删除当前会话。 |
| `PATCH` | `/api/auth/password` | 教师 | 使用 `{ currentPassword, newPassword }` 修改密码，并注销全部会话。 |

### 扫码登录

电脑端调用 `POST /api/auth/qr/start`，返回短期 `token`（二维码内容）、
`claimToken`（电脑轮询用）和 `expiresAt`。手机打开二维码中的 `/qr-login?token=...`，
可调用下列接口完成确认：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/auth/qr/start` | 请求体 `{ deviceName? }`，创建 2 分钟有效的登录请求。 |
| `GET` | `/api/auth/qr/challenge?token=...` | 手机读取设备名称和状态，并将状态置为 `scanned`。 |
| `POST` | `/api/auth/qr/decision` | 手机提交 `{ token, approve }`，确认或拒绝登录。 |
| `POST` | `/api/auth/qr/status` | 电脑提交 `{ token: claimToken }`；批准后写入教师 Cookie。 |

二维码状态依次可能为 `pending`、`scanned`、`approved`、`denied`、`consumed`、`expired`。

## 教师工作台

### 概览、班级和积分

| 方法 | 路径 | 请求/查询参数 | 返回 |
| --- | --- | --- | --- |
| `GET` | `/api/dashboard` | — | `classCount`、`studentCount`、`totalScore`、`activeSessions`。 |
| `GET` | `/api/classes` | — | `ClassItem[]`，包含学生数、总积分和当前考勤场次。 |
| `POST` | `/api/classes` | `{ grade, classNo }` | 创建或恢复班级。班级名称由年级和班号生成。 |
| `PATCH` | `/api/classes/{id}` | `{ grade, classNo }` | 修改班级并同步历史快照显示名。 |
| `DELETE` | `/api/classes/{id}` | — | 软删除班级。 |
| `GET` | `/api/classes/{id}/students?sort=student_no\|score_asc\|score_desc` | — | 当前学生名单。 |
| `POST` | `/api/classes/{id}/students` | `{ studentNo, name }` | 新增学生。 |
| `PATCH` | `/api/students/{id}` | `{ studentNo, name }` | 修改学生资料。 |
| `DELETE` | `/api/students/{id}` | — | 软删除学生。 |
| `POST` | `/api/classes/{id}/import` | multipart `file` | 导入 Excel；返回 `added/restored/skipped/total`。 |
| `GET` | `/api/students/{id}/events` | — | 最近 50 条积分流水。 |
| `POST` | `/api/students/{id}/score` | `{ delta, reason? }` | 调整积分，单次 `-100..100` 且不为 0。 |
| `POST` | `/api/score-events/{id}/undo` | — | 以反向流水撤销可撤销积分。 |

### 考勤

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/attendance?class_id=&date=&trash=&cursor=&limit=` | 游标分页列表；`trash=true` 查看回收站。 |
| `POST` | `/api/attendance` | `{ classId, course?, sessionAt? }`，发起场次。 |
| `GET` | `/api/attendance/{id}` | 返回场次、统计和学生快照。 |
| `POST` | `/api/attendance/{id}/close` | 结束场次并确认缺席名单。 |
| `DELETE` | `/api/attendance/{id}` | 移入回收站。 |
| `POST` | `/api/attendance/{id}/restore` | 从回收站恢复。 |
| `DELETE` | `/api/attendance/{id}/permanent` | 永久删除场次及记录。 |
| `PATCH` | `/api/attendance/{id}/records/{studentID}` | `{ status: present\|absent\|late\|leave }`，教师修正状态。 |

### 课程表

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/schedule` | 返回 `lessons`、`changes`、`settings`。 |
| `GET` | `/api/schedule/current` | 按服务器时间返回当前课时建议；教师仍需确认。 |
| `POST` | `/api/schedule` | 创建课程：`classId/course/weekday/period/locationOdd/locationEven`。 |
| `PATCH` | `/api/schedule/{id}` | 修改课程。 |
| `DELETE` | `/api/schedule/{id}` | 删除课程。 |
| `POST` | `/api/schedule/import` | multipart `file` 导入课表 Excel。 |
| `PUT` | `/api/schedule/settings` | 更新学期日期和 7 节课时间。 |
| `PUT` | `/api/schedule/{id}/changes` | 设置某日 `occupied` 或 `rescheduled`。 |
| `DELETE` | `/api/schedule/{id}/changes?date=YYYY-MM-DD` | 删除某日课表变动。 |

### 品牌、导航和教学网页

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/settings` | 读取教师端站点标题。 |
| `PATCH` | `/api/settings` | `{ title, subtitle }`，保存站点标题。 |
| `GET` | `/api/navigation` | 读取完整导航列表及教学网页元数据。 |
| `PUT` | `/api/navigation` | `{ items: NavigationItem[] }`，原子替换排序和外部链接。 |
| `POST` | `/api/navigation/sites` | multipart 上传 HTML/文件夹/ZIP 教学网页。 |
| `GET` | `/api/navigation/sites/{id}` | 读取单个教学网页项目。 |
| `PUT` | `/api/navigation/sites/{id}` | multipart 上传新版本。 |
| `DELETE` | `/api/navigation/sites/{id}` | 删除教学网页和缓存文件。 |

上传教学网页限制：单个压缩源 32 MiB，解压后 128 MiB，最多 2000 个文件；必须包含
`index.html`。静态网页统一通过 `/published/{publicId}/{revision}/...` 提供，revision 为内容版本。
旧 `/published-v2/{publicId}/{revision}/...` 返回 308 到规范地址，保留资源路径及查询参数；它不保存或提供另一套网页。

## 学生公开接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/public/classes` | 返回当前有开放考勤的班级。 |
| `GET` | `/api/public/settings` | 返回公开站点标题。 |
| `GET` | `/api/public/navigation` | 返回学生学习导航。 |
| `GET` | `/api/public/classes/{id}/students` | 返回当前场次和可签到学生，含 `checkedIn`。 |
| `POST` | `/api/public/check-in` | `{ classId, studentId }`；重复签到返回 `409`。 |

## 管理与运维接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/admin/audit-logs?limit=&before_id=` | 读取倒序审计日志，默认 30 条，最多 100 条。 |
| `GET` | `/api/admin/backup` | 下载一致性 SQLite 备份，响应为文件流。 |
| `POST` | `/api/admin/restore` | multipart `file` 恢复备份；恢复前自动生成安全副本。 |
| `GET` | `/api/admin/reports?type=roster\|attendance\|scores&class_id=&from=&to=` | 下载 Excel 报表。考勤报表必须提供日期范围。 |

## 外部班级名单集成

`GET /api/integration/classes` 不使用浏览器 Cookie，而使用部署配置中的
`CLASS_SYSTEM_TOKEN`：

```http
Authorization: Bearer <CLASS_SYSTEM_TOKEN>
GET /api/integration/classes?teacher_username=teacher
```

响应只包含班级、稳定班级 ID、学生学号和姓名，不包含积分、考勤、密码或会话。支持
`ETag`/`If-None-Match`；名单未变化时返回 `304`。完整字段、错误码、安全要求和 curl 示例见
[`integration-classes-api.md`](integration-classes-api.md)。

## 修改接口时的约束

1. 新增或修改 JSON 字段时，同时更新 `backend/models.go`、`frontend/src/types.ts` 和本文档。
2. 数据库字段只能通过追加式迁移增加，迁移写入 `backend/migrations.go` 并补充旧库测试。
3. 外部可见行为变更必须补充前后端测试；删除或恢复操作要说明审计与回滚策略。
4. 不要把数据库内部字段、教师密码哈希或共享 Token 放进公开响应。
