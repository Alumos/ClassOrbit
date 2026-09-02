# 更新日志

本项目的显著变更都记录在这里。版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)，记录格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

## [1.5.0] - 2026-09-02

### Added

- 班级列表新增直达的可视化名单维护入口，可在同一界面新增插班生、编辑或删除学生。

### Changed

- 操作审计默认折叠，并改为每页 10 条的游标分页，避免设置页面被日志持续拉长。

## [1.4.0] - 2026-09-02

### Added

- 学生名单导入兼容 Excel 97–2003 `.xls` 文件，并继续支持 `.xlsx` 与 `.xlsm`。

## [1.3.0] - 2026-08-31

### Changed

- 全站改用统一楷体字体栈，并与 KeySprint 共用 11–20px 的界面字号层级。
- 对齐侧栏、顶栏、按钮、表单、表格、卡片、弹窗及课表等界面的排版尺度。

## [1.1.0] - 2026-08-30

### Added

- KeySprint 名单接口为每个班级返回稳定的字符串 ID，班级改名后仍能保持历史关联。

### Changed

- 全面更新 KeySprint 联动名称、部署说明、接口示例与类型文档。

## [1.0.0] - 2026-08-30

### Added

- 班级与学生名单管理、Excel 导入、课堂积分、随机点名和可审计积分撤销。
- 学生自助签到、教师考勤台、考勤回收站、历史名单快照和 Excel 报表。
- 服务器时间驱动的当前课时识别，兼容校历、节次、单双周地点、占课和临时换课，并要求教师人工确认。
- KeySprint 班级名单只读接口，支持独立 Bearer Token、ETag 和 `304 Not Modified`。
- 教师账号初始化、密码修改、会话撤销、操作审计、公开接口限流和安全响应头。
- SQLite 版本迁移、一致性备份与恢复、每日自动备份和保留策略。
- Docker Compose 部署、GHCR 多架构镜像、SBOM、构建来源和 GitHub Actions 自动发布。

### Security

- 教师 API 与学生公开 API 隔离，密码使用 bcrypt 存储，登录会话只保存 Token 哈希。
- npm 依赖审计在发布时无已知漏洞。

[Unreleased]: https://github.com/Alumos/ClassOrbit/compare/v1.5.0...HEAD
[1.5.0]: https://github.com/Alumos/ClassOrbit/releases/tag/v1.5.0
[1.4.0]: https://github.com/Alumos/ClassOrbit/releases/tag/v1.4.0
[1.3.0]: https://github.com/Alumos/ClassOrbit/releases/tag/v1.3.0
[1.1.0]: https://github.com/Alumos/ClassOrbit/releases/tag/v1.1.0
[1.0.0]: https://github.com/Alumos/ClassOrbit/releases/tag/v1.0.0
