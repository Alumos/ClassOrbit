# ClassOrbit 开发与发布规范

ClassOrbit 保持单体、轻量和可恢复的架构。功能开发应优先保证课堂使用稳定、数据结构清晰和 SQLite 事务一致性。

## 分支与提交

`main` 始终保持可发布状态，不直接在已经发布的 Tag 上修改历史。日常工作从最新 `main` 创建短生命周期分支：

- `feat/<name>`：新增功能。
- `fix/<name>`：缺陷修复。
- `refactor/<name>`：不改变外部行为的重构。
- `docs/<name>`：仅文档调整。
- `chore/<name>`：依赖、CI 或工程维护。

提交信息采用 Conventional Commits 的核心格式：

```text
feat: add attendance recycle bin
fix: keep empty classes in integration roster
docs: document release process
```

常用类型为 `feat`、`fix`、`refactor`、`perf`、`test`、`docs`、`build`、`ci` 和 `chore`。一次提交只解决一类问题，不提交 `.env`、数据库、真实学生数据、Token 或密码。

## 合并前检查

```bash
make test
make release-check
git diff --check
```

Pull Request 至少说明修改目的、数据迁移影响、验证方式和部署注意事项。涉及数据库结构时必须增加追加式迁移和旧库升级测试；涉及删除或恢复时必须说明审计与回滚策略。

## 版本号

项目使用稳定 SemVer：`MAJOR.MINOR.PATCH`。

- `MAJOR`：存在需要人工处理的不兼容 API、配置或数据升级。
- `MINOR`：向后兼容的新功能。
- `PATCH`：向后兼容的缺陷、安全或性能修复。

正式版本必须同时更新：

1. 根目录 `VERSION`。
2. `frontend/package.json` 和 `frontend/package-lock.json` 中的项目版本。
3. `CHANGELOG.md`，把 `[Unreleased]` 内容整理到带日期的版本标题下。

Tag 固定使用带 `v` 前缀的形式，例如 `v1.2.3`。发布检查会拒绝与 `VERSION` 不一致的 Tag。

## 发布步骤

1. 从最新 `main` 创建 `chore/release-vX.Y.Z`。
2. 更新三个版本位置和 `CHANGELOG.md`，运行 `make test && make release-check`。
3. 合并发布提交到 `main`，确认主分支 Actions 通过。
4. 创建带说明的 Tag：`git tag -a vX.Y.Z -m "ClassOrbit vX.Y.Z"`。
5. 推送 Tag：`git push origin vX.Y.Z`。
6. GitHub Actions 校验版本、运行测试、构建 `linux/amd64` 与 `linux/arm64` 镜像，并发布 GHCR 标签 `X.Y.Z`、`X.Y`、`sha-*`。
7. 镜像发布成功后，Actions 自动创建同名 GitHub Release。

不要移动或覆盖已经公开的版本 Tag。发布失败时修复代码并递增版本；只有从未成功公开的误操作 Tag 才可在团队确认后删除。
