# Sub2API Fork 维护契约

## 目的与适用范围

本文档面向 fork 维护者和 AI 编码代理，只记录相对已合并上游基线仍然生效、且有意保留的行为差异。

- 新增、修改或删除 fork 行为时，必须在同一提交更新对应条目。
- 对用户可见的 fork 能力，还必须同步英文 `README.md`、中文 `README_CN.md`、日文 `README_JA.md` 及其索引；仓库引入 `CHANGELOG.md` 后同时维护当前未发布段。纯维护者内部差异只写入本文档。
- 只有已合并上游提供等价行为，并且回归测试覆盖等价时，才能删除对应条目。
- 上游合并审查以本文档中的行为不变量为准，不以冲突文件归属、字段同名或“选 ours/theirs”作为判断标准。
- 生成文件不是行为来源。Ent、Wire 等输出发生冲突时，应先解决 schema、migration、provider 或 wire source，再重新生成。
- 单独执行 fork 审计时，本文档默认保持未提交；若审计属于已获授权且尚未完成的 merge、rebase 或 feature commit，则必须随同一逻辑提交写入。推送和发布仍需单独明确执行。

## 精确上游基线

| 项目 | 值 |
| --- | --- |
| Fork 分支 | `stable` |
| 权威上游 | `upstream` -> `git@github.com:Wei-Shaw/sub2api.git` |
| 上游默认分支 | `main` |
| 已合并上游提交 / 比较基线 | `200602b41bf97c706f8c28fdc9df97ef5ece1aa9` |
| 当前比较范围 | `200602b41bf97c706f8c28fdc9df97ef5ece1aa9..HEAD` |

`upstream/main` 是移动目标，不自动等于本文档基线。本次合并固定上游提交为 `200602b41bf97c706f8c28fdc9df97ef5ece1aa9`；远端后续推进不改变本次 merge 的第二父，下一次审计仍须从最新已合并上游 merge 重新确定基线。

## Fork 发布版本

| 项目 | 值 |
| --- | --- |
| 权威上游版本源 | 上游父提交中的 `backend/cmd/server/VERSION` |
| Fork 版本源 | `backend/cmd/server/VERSION` |
| 当前上游版本 | `0.1.184` |
| 当前 Fork 版本 | `0.1.183-fork.4` |
| 已发布同基线 Fork 版本 | `v0.1.183-fork.4`（版本基数与当前上游基线不一致） |
| 下次发布所需版本 | `0.1.184-fork.1` |

所有 fork release 必须使用 `<upstream-version>-fork.<N>`。上游版本变化时从 `fork.1` 重新开始；同一上游版本的后续 fork release 从已发布的最高 `N` 递增，不得以 plain upstream version 发布 fork 构建。

发布流程已将版本源同步为 `0.1.183-fork.4`，但当前上游基线版本已是 `0.1.184`，该 release 不符合本契约的版本基数要求。下次发布前必须将版本源提升为 `0.1.184-fork.1`；本次 CI 测试修复不修改 VERSION，也不重写既有 tag。

## 能力索引

| # | 能力 | 生命周期 |
| --- | --- | --- |
| 1 | 全局管理员角色与细粒度权限 | `长期保留` |
| 2 | 实际调度分组归因 | `长期保留` |
| 3 | 账号数据交换与批量账号操作 | `长期保留` |
| 4 | Fork 发布、版本与更新通道 | `长期保留` |
| 5 | 提示词审计的数据最小化与授权 | `长期保留` |
| 6 | 非 Codex 客户端的 OpenAI 图片策略 | `长期保留` |
| 7 | OpenAI/Codex 协议、故障转移与账号状态正确性 | `等待上游吸收` |
| 8 | Grok/xAI OAuth、协议与故障转移正确性 | `等待上游吸收` |
| 9 | 鉴权、会话绑定与审计状态正确性 | `等待上游吸收` |
| 10 | 支付定价、结果 DTO 与退款状态机正确性 | `等待上游吸收` |
| 11 | 用量与运维可观测性、查询性能 | `等待上游吸收` |
| 12 | 管理端交互与紧凑诊断体验 | `等待上游吸收` |
| 13 | 相对部署、示例配置与迁移恢复 | `等待上游吸收` |

## 1. 全局管理员角色与细粒度权限

- **生命周期**：`长期保留`
- **原始意图**：取消面向用户的 Project 空间、成员和资源隔离模型；保留 `super_admin`、`admin`、`user` 三种角色，并把管理员功能权限和分组、账号、代理、订阅四类资源范围统一配置在用户管理列表。
- **行为不变量**：`super_admin` 始终拥有全部后台权限，且不能通过管理员权限接口修改；`admin` 只拥有用户记录 `admin_permissions` 中显式授予的 8 项白名单权限，并按 `all/restricted` 资源模式访问四类管理资源；`user` 的管理员权限和资源范围必须为空。受限模式的分组、账号、代理、订阅均为独立直接绑定，勾选分组不得隐式开放分组内账号，所有列表、详情、汇总和批量操作都必须与对应直接绑定取交集；受限管理员新建资源后自动绑定给自己。创建用户和普通用户编辑接口不得提升或降级角色，只有超级管理员完成 step-up 验证后，才能通过 `PUT /api/v1/admin/users/:id/admin-access` 在同一事务内修改其他用户的 `admin/user` 角色、功能权限和资源范围；无效资源 ID 必须使整次更新回滚，修改后必须立即失效鉴权缓存。该资源范围不是 Project 空间：API Key、用量、日志、批量图片任务、Dashboard、Ops 和调度缓存仍不得读取 Project context、成员关系、Profile binding、`X-Project-ID` 或前端选中项目状态。
- **权限集合**：`admin.dashboard.read`、`admin.ops.read`、`admin.accounts.write`、`admin.users.manage`、`admin.groups.manage`、`admin.proxies.manage`、`admin.subscriptions.manage`、`admin.usage.read`。写入时必须校验白名单、去重并排序；未知项和空项必须拒绝。
- **当前代码**：`backend/ent/schema/user.go`、`backend/internal/repository/legacy_project_id.go`、`backend/internal/repository/admin_resource_scope_repo.go`、`backend/internal/service/permission_service.go`、`backend/internal/service/admin_user.go`、`backend/internal/handler/admin/access_scope.go`、`backend/internal/handler/admin/user_handler.go`、`backend/internal/server/middleware/admin_auth.go`、`backend/internal/server/middleware/admin_permission.go`、`backend/internal/server/routes/admin.go`、`frontend/src/constants/adminPermissions.ts`、`frontend/src/views/admin/UsersView.vue`。
- **迁移与测试**：`backend/migrations/233_add_user_admin_permissions.sql`、`backend/migrations/234_backfill_user_admin_permissions.sql`、`backend/migrations/236_admin_resource_scopes.sql`、`backend/migrations/user_admin_permissions_migration_test.go`、`backend/migrations/admin_resource_scope_migration_test.go`、`backend/internal/repository/legacy_project_id_test.go`、`backend/internal/repository/admin_resource_scope_repo_test.go`、`backend/internal/service/admin_service_role_test.go`、`backend/internal/service/admin_service_bulk_update_test.go`、`backend/internal/handler/admin/operator_account_scope_test.go`、`backend/internal/handler/admin/group_summary_scope_test.go`、`backend/internal/handler/admin/subscription_scope_test.go`、`backend/internal/server/middleware/admin_auth_test.go`、`backend/internal/server/routes/admin_permission_routes_test.go`、`frontend/src/api/__tests__/admin.users.spec.ts`、`frontend/src/views/admin/__tests__/UsersView.spec.ts`。
- **旧 Project 存储兼容**：本次不物理删除 `projects`、`project_members`、`project_profiles`、`project_profile_bindings` 及资源表中的 `project_id`，Ent schema 也暂时保留，避免旧列 `NOT NULL`、外键和混合版本部署阻断写入。兼容代码只能为新资源补默认 `project_id`，不得创建成员、绑定或恢复范围查询。确认所有部署已升级并完成数据审计后，必须另做独立 contract migration 删除旧表、列、索引、外键、Ent schema 和兼容写入。
- **合并审查**：搜索新增 admin route、repository query、批量/汇总入口、cache key、scheduler snapshot、WebSocket 和后台原始 `fetch`；管理资源入口必须执行对应直接绑定范围，且账号范围不得从分组成员关系推导。任何重新引入 Project context、成员检查、Profile scope、项目请求头或项目切换 UI 的改动，都应视为与本能力冲突。上游若新增权限点，必须明确映射到现有 8 项权限或经维护者批准扩展白名单。
- **删除条件**：只有上游提供等价的全局角色、权限白名单、独立管理员权限入口、step-up 和缓存失效契约，并有同等回归覆盖时才可删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test.*AdminAccess|TestNormalizeAdminPermissions')
(cd backend && go test ./internal/repository ./internal/handler/admin ./migrations -run 'Test.*(AdminResource|RestrictedAdmin|GroupSummaries)')
(cd backend && go test -tags=unit ./internal/server/middleware ./internal/server/routes -run 'Test.*AdminPermission|Test.*AdminAccess')
(cd backend && go test ./migrations -run '^TestUserAdminPermissionsMigrations$')
(cd frontend && ./node_modules/.bin/vitest run src/api/__tests__/admin.users.spec.ts src/views/admin/__tests__/UsersView.spec.ts)
```

## 2. 实际调度分组归因

- **生命周期**：`长期保留`
- **原始意图**：确保 fallback/composite 调度后的计费、并发、sticky session、利润准入、推理策略、配额平台、Cyber policy 和用量日志归因符合各自契约。
- **行为不变量**：真实 fallback 后的检查、利润准入、倍率、订阅计费、账号统计和用量日志必须使用 `AccountSelectionResult` 的有效分组，不得回退到原 `apiKey.Group`；composite 只路由目标平台、不携带成员分组关系，因此 selection 与 usage 继续归属于 API Key 所属父分组，且 composite 本身不得凭空安装成员分组利润门；同一请求共享一个 `PricingAt`。API Key 自身的 5 小时限额继续独立生效，不属于本能力。
- **当前代码**：`backend/internal/service/gateway_request_pricing.go`、`backend/internal/service/gateway_profit_control.go`、`backend/internal/service/openai_profit_control.go`、`backend/internal/service/gateway_usage_billing.go`、`backend/internal/service/openai_gateway_usage.go`、`backend/internal/handler/gateway_handler.go`、`backend/internal/handler/openai_gateway_handler.go`、`backend/internal/handler/endpoint.go`。
- **迁移与测试**：`backend/migrations/145_group_5h_rate_limits.sql` 仅保留为已执行历史；`backend/migrations/235_drop_group_5h_rate_limits.sql` 删除分组 5 小时字段和窗口表，`backend/migrations/group_5h_rate_limit_removal_migration_test.go` 锁定删除顺序并确认 API Key 5 小时限额仍存在。实际分组归因由 `backend/internal/handler/endpoint_test.go`、`backend/internal/service/gateway_record_usage_test.go`、`backend/internal/service/openai_gateway_record_usage_test.go`、`backend/internal/service/gateway_profit_control_v2_test.go`、`backend/internal/service/openai_profit_control_paths_test.go` 和 `backend/internal/service/response_model_billing_test.go` 覆盖。
- **来源提交**：`a80e366bbe27d3212c68ae028cf54cbd714dbe69`、`a6e3a1ceede4cdc048e1f471990b82cc406dd001`、`bbe433256021676ab389439d3bb8157cb0662372`、`7b51c2bd4b53d669d5c37aedaaa9f0ce41edf7df`。
- **当前变更定位**：搜索 `ResolveEffectiveGroupID`、`ResolveEffectiveGroup`、`EffectiveGroupID` 和 `EffectiveGroup` 可定位实际分组从 handler 到 usage/billing 的传递；`git log -S'composite 请求即父分组' -- backend/internal/service/openai_profit_control.go` 可定位 composite 利润门契约。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309`、`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 和 `a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 均保留了实际选中分组对策略、倍率、订阅计费、配额平台和用量归因的决定权；原分组 5 小时预检与记账已由 migration `235` 及本次代码删除移除。
- **合并审查**：重点检查 gateway 预检、账户切换、sticky binding、usage worker 和图像/Grok 分支；同名 `group_id` 不代表已使用实际调度分组。
- **删除条件**：不主动删除。只有上游提供等价的实际调度分组归因，并有同等回归覆盖时才可移除。
- **聚焦验证**：

```bash
(cd backend && go test ./migrations -run '^TestGroup5hRateLimitRemovalMigration$')
(cd backend && go test -tags=unit ./internal/handler -run '^TestResolveEffectiveGroup')
(cd backend && go test -tags=unit ./internal/service -run 'Test(GatewayProfitControlCompositeSelectionKeepsParentGroupWithoutSyntheticGate|GatewayProfitControlFallbackUsesResolvedGroupRate|ProfitControl_CompositeSelectionKeepsParentGroupWithoutSyntheticGate)')
(cd backend && go test -tags=unit ./internal/service -run 'Test(GatewayServiceRecordUsage|OpenAIGatewayServiceRecordUsage)')
```

## 3. 账号数据交换与批量账号操作

- **生命周期**：`长期保留`
- **原始意图**：支持 native/CPA 账号数据导入导出、文件级错误定位、批量连接测试、按账号选择模型、最近测试跳过和按失败类型清理账号。
- **行为不变量**：导入不得留下半成功账号或破坏既有代理；CPA schema 自动识别且敏感字段不泄漏；批量测试必须限制并发；只有已加载的完整账号集合可进入测试或删除，失败删除结果必须逐项对账。批量操作不得注入项目请求头；受限管理员的账号批量操作还必须与直接账号绑定取交集，分组绑定不能扩大账号范围。
- **当前代码**：`backend/internal/handler/admin/account_data.go`、`backend/internal/handler/admin/account_data_cpa.go`、`frontend/src/components/admin/account/ImportDataModal.vue`、`frontend/src/components/admin/account/AccountBatchTestModal.vue`、`frontend/src/components/admin/account/AccountBulkActionsBar.vue`、`frontend/src/utils/accountTestRunner.ts`、`frontend/src/views/admin/AccountsView.vue`。
- **测试**：`backend/internal/handler/admin/account_data_handler_test.go`、`frontend/src/__tests__/integration/data-import.spec.ts`、`frontend/src/components/admin/account/__tests__/AccountBatchTestModal.spec.ts`、`frontend/src/components/admin/account/__tests__/AccountBulkActionsBar.spec.ts`、`frontend/src/views/admin/__tests__/AccountsView.batchTest.spec.ts`。
- **来源提交**：`3a295166d17636c9f3b44e53c47a7804ab83819e`、`454639e07532e35a0823048ad18948b508554615`、`3543db03bbcf1750852642115f48b61f7b61bac9`、`affd89e9f8f5c49b0f9909824c779187782ae5ab`、`67a183600a13e8a0170bf6780e2fb60ee3e9501b`、`a6ab6d15cab3d5fb2dcc2d2c65da1c6f6400625b`。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309` 保留 `allSelectedAccountsLoaded` 测试门禁、批量删除结果对账和既有批量测试入口；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 接入上游 Grok 图片、音频和视频测试字段。该提交中原有的 `X-Project-ID` 解决已随项目空间移除而失效，后续不得恢复。
- **合并审查**：同时检查 backend import/export DTO、frontend file parser、bulk selection 和 raw SSE `fetch`；不能只验证单账号弹窗。
- **删除条件**：不主动删除。只有维护者明确取消 CPA/批量运维工作流时才可移除。
- **聚焦验证**：

```bash
(cd backend && go test ./internal/handler/admin -run 'Test(ExportData|ImportData)')
(cd frontend && ./node_modules/.bin/vitest run src/__tests__/integration/data-import.spec.ts src/components/admin/account/__tests__/AccountBatchTestModal.spec.ts src/views/admin/__tests__/AccountsView.batchTest.spec.ts)
```

## 4. Fork 发布、版本与更新通道

- **生命周期**：`长期保留`
- **原始意图**：从 `stable` 构建 fork 二进制和镜像，复用主 release pipeline，同时保持登录身份、发布目标、rolling tag 和更新检查彼此独立。
- **行为不变量**：Fork release 使用 `vX.Y.Z-fork.N`；发布 base 必须取自当前检出代码的 `backend/cmd/server/VERSION` 并验证对应上游稳定 tag，不得取远端最新 tag 替代尚未合并的版本；上游提升 base version 时立即使用新 base 的 `fork.1`，同一 base 后续发布递增 `N`；相同 base version 下 plain release 小于 fork release；只有当前运行版本本身属于 fork channel 时，rollback 才可接受 GitHub 标为 prerelease 的严格 `vX.Y.Z-fork.N`，不得混入 rc/beta 或让 plain upstream channel 跨入 fork；`DOCKERHUB_USERNAME` 只表示登录身份，发布目标由 `DOCKERHUB_NAME`/`DOCKERHUB_IMAGE` 决定；fork release 不更新上游 rolling tags。
- **当前代码**：`.github/workflows/stable-fork-release.yml`、`.github/workflows/release.yml`、`.goreleaser.yaml`、`backend/cmd/server/VERSION`、`backend/internal/service/update_service.go`、`backend/internal/repository/github_release_service.go`、`deploy/install.sh`。
- **测试**：`backend/internal/service/update_service_test.go`、`backend/internal/repository/github_release_service_test.go`。
- **来源提交**：`c6af7d1ce7c5a3962e283aa7dd843e5602fd6e74`、`59af010693739c41834f1f5e16d1c4e564abefb6`、`4a7594f2f756dd6c60cfcbd425509dc94e740cb5`、`5ad3a11a1c46561c82a9a9452d8884bfe6423a54`。
- **当前变更定位**：运行 `git log -S'forkPart := strings.TrimPrefix' -- backend/internal/service/update_service.go`，可定位同时支持任意连字符后缀与 `-fork.N` revision 比较的兼容提交。
- **人工合并解决**：合入 `ac18c588c81821b3c4fd4f2c2457dd9a3e341737` 后，独立功能提交保留同 base 下 fork revision 的顺序，并吸收上游对 `-custom` 等普通连字符后缀的解析；VERSION-only 提交只是该策略的派生元数据。
- **合并审查**：先判断上游是否 bump base version；若已 bump，版本源改为新 base 的 `fork.1`；否则按该 base 已发布的最高 fork revision 递增。不得把 registry login 名重新当作镜像 namespace。
- **删除条件**：不主动删除。只有 fork 停止独立发布和更新时才可移除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test(UpdateService|CompareVersions)')
(cd backend && go test ./internal/repository -run '^TestGitHubRelease')
actionlint .github/workflows/stable-fork-release.yml
```

## 5. 提示词审计的数据最小化与授权

- **生命周期**：`长期保留`
- **原始意图**：提示词审计只持久化派生、脱敏结果，不保存原始提示词；审计入口仅超级管理员可见；Qwen3Guard 返回必须完整、结构化且可验证。
- **行为不变量**：数据库和 API DTO 均不得重新出现完整 prompt；普通 admin 不得访问审计路由；缺字段、未知类别、非法 safety/refusal 或多结果不完整时必须 fail closed；清理操作不得删除预览之后的新事件。
- **当前代码**：`backend/internal/securityaudit/prompt_repository.go`、`backend/internal/securityaudit/prompt_event_repository.go`、`backend/internal/securityaudit/prompt_worker.go`、`backend/internal/securityaudit/prompt_qwen3guard.go`、`backend/internal/server/routes/admin.go`、`frontend/src/features/prompt-audit/`。
- **迁移与测试**：`backend/migrations/183_drop_prompt_audit_full_prompt.sql`、`backend/migrations/prompt_audit_privacy_migration_test.go`、`backend/internal/securityaudit/prompt_repository_integration_test.go`、`backend/internal/securityaudit/prompt_qwen3guard_test.go`、`backend/internal/server/routes/prompt_audit_route_coverage_test.go`。
- **来源提交**：`0a6eb610c1f28b738a0b56837ecb44432b6fe739`、`2908160738b82925d3b10854c62092beead2da37`、`86484c52138a4ddabb5ffd0dfcc3791a068202be`。
- **上游部分吸收**：`1b04e03cc4c7c23c216ae0f4830b593700b06eda` 已增加 Responses `output_text` 解析及回归，`c418fd522f429e80c5606d90393d7da601ca30d5` 已增加 WebSocket 最新 turn 审计去重，`f6aa9dc3c` 又让周期性 prompt audit 配置刷新只在首次加载、配置变化或错误恢复时写日志；它们都不覆盖原始 prompt 不落库、路由授权、Qwen3Guard 完整性 fail closed 和并发清理契约，因此本能力继续保留。
- **人工合并解决**：无相关人工解决锚点。
- **合并审查**：任何 schema、event snapshot、API response 或前端详情字段新增都要检查原始 prompt 泄漏；解析器不可用“缺省 Safety”代替验证。
- **删除条件**：不主动删除。即使上游实现类似审计，也必须先证明数据最小化、路由授权和 fail-closed 测试等价。
- **聚焦验证**：

```bash
(cd backend && go test ./internal/securityaudit -run 'Test(ParseQwen3Guard|PromptAudit)')
(cd backend && go test ./internal/server/routes -run '^TestPromptAudit')
```

## 6. 非 Codex 客户端的 OpenAI 图片策略

- **生命周期**：`长期保留`
- **原始意图**：允许账号级图片策略显式作用于非 Codex Responses 客户端，同时保持官方 Codex 客户端的自动注入行为。
- **行为不变量**：非 Codex 客户端只有账号 opt-in 时才能启用图片 bridge；显式 strip 策略必须删除图片工具；group image permission 始终优先；`/responses/compact` 不注入图片工具。
- **当前代码**：`backend/internal/service/openai_gateway_forward.go`、`backend/internal/service/openai_codex_transform.go`、`backend/internal/service/openai_images.go`、`backend/internal/handler/openai_images.go`。
- **测试**：`backend/internal/service/openai_image_generation_controls_test.go`、`backend/internal/handler/openai_images_controls_test.go`。
- **来源提交**：`d43bbf80f6ad4dd05cd704036ecce940f4c2def9`。
- **人工合并解决**：无相关人工解决锚点。
- **合并审查**：区分 client identity、account policy、group permission 和 endpoint type；任何一层同名开关都不能替代其他层。
- **删除条件**：不主动删除；只有产品取消非 Codex 图片策略时才可移除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'TestOpenAIGatewayServiceForward_(NonCodexImageBridgeRequiresAccountOptIn|AccountPolicyStripsExplicitImageTool)')
(cd backend && go test -tags=unit ./internal/handler -run '^TestOpenAIGatewayHandlerImages_')
```

## 7. OpenAI/Codex 协议、故障转移与账号状态正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：修复 Codex reasoning/Agent Identity、OpenAI privacy、Alpha Search、Responses passthrough、proxy stream circuit、429 清理和 account failover 的通用正确性。
- **行为不变量**：保留可回放的 encrypted reasoning，剥离不可回查引用；Agent Identity 只恢复一次且不泄漏 assertion；代理 quarantine 只阻断对应请求范围并维持 fail-open 语义；额度重置的上游成功是不可逆结果，本地 429/runtime block 清理失败必须进入 HTTP 200 的部分成功恢复/告警流程，不得返回可重试失败并再次消耗 reset credit；`RequestScopedTransient` 只能驱动请求级重试/failover，不得降低所选账号的 scheduler health；失效 OAuth 账号不能阻断切换；Responses namespace 和 compact/passthrough 决策保持协议一致；passthrough 外部取消必须先选择精确 close code，关闭客户端连接以解除阻塞写，并在 `Relay` 返回前 join 已启动的 relay worker；ingress lease loss 在连接可写时保持 1013。
- **Ultrafast 协议不变量**：Responses、Chat Completions 与 Responses WebSocket 必须把官方 `service_tier: "ultrafast"` 原样转发，不能转换为 `priority`，也不在 Sub2API 硬编码模型白名单；Fast Policy 可单独匹配 Ultrafast，`force_priority` 仍明确表示主动降为普通 Fast；私有 `x-codex-routing-hint` 暂不扩展 Ultrafast。
- **Ultrafast 实现与文档**：代码位于 `backend/internal/service/openai_gateway_request_body.go`、`backend/internal/service/settings_view.go`、`backend/internal/service/setting_features.go`、`frontend/src/views/admin/SettingsView.vue`；测试位于 `backend/internal/service/openai_fast_service_tier_test.go`、`backend/internal/service/openai_gateway_chat_completions_test.go`、`backend/internal/service/openai_fast_policy_ws_test.go`；用户文档为 `README.md`、`README_CN.md`、`README_JA.md`。提交后可运行 `git log -S'OpenAIFastTierUltrafast' -- backend/internal/service/settings_view.go` 定位当前变更。
- **限额错误不变量**：选定分组后的二次计费检查遇到 subscription 日、周、月用量耗尽时，必须保持 HTTP 429 和 `rate_limit_exceeded`，不得降级为 403；认证前置路径继续使用既有 `USAGE_LIMIT_EXCEEDED` 协议。
- **限额错误映射**：代码位于 `backend/internal/handler/gateway_handler.go`，由 `backend/internal/handler/endpoint.go` 等入口共用；回归测试位于 `backend/internal/handler/gateway_handler_billing_error_test.go`；用户文档为 `README.md`、`README_CN.md`、`README_JA.md`。
- **当前修复定位**：提交后运行 `git log -S'pkgerrors.IsTooManyRequests' -- backend/internal/handler/gateway_handler.go`、`git log -S'FixedRequestModel string' -- backend/internal/service/openai_ws_forwarder.go` 与 `git log -S'acceptedTurnStartedAt.Swap(nil)' -- backend/internal/service/openai_ws_v2_passthrough_adapter.go`。
- **当前基线审查**：当前上游基线 `fdf9751c1223a74a7153e537c6d9d1fb14ee9cad` 已包含 `f1aadd48d`，让 OpenAI OAuth 上游配额耗尽 429 暂停账号，但仍未覆盖 subscription 日、周、月限额的通用 429 映射；fork 继续通过 `pkgerrors.IsTooManyRequests` 保持该协议。
- **当前代码**：`backend/internal/service/openai_codex_transform.go`、`backend/internal/service/openai_agent_identity.go`、`backend/internal/service/openai_privacy_service.go`、`backend/internal/util/httputil/httputil.go`、`backend/internal/handler/openai_alpha_search.go`、`backend/internal/service/openai_alpha_search.go`、`backend/internal/service/openai_account_scheduler.go`、`backend/internal/service/openai_proxy_stream_circuit.go`、`backend/internal/service/openai_account_runtime_block_fastpath.go`、`backend/internal/service/openai_quota_service.go`、`backend/internal/handler/admin/openai_oauth_handler.go`、`backend/internal/service/gateway_service.go`、`backend/internal/service/openai_gateway_forward.go`、`backend/internal/service/openai_ws_v2/passthrough_relay.go`、`backend/internal/service/openai_ws_v2_passthrough_adapter.go`、`backend/internal/handler/openai_gateway_handler.go`、`frontend/src/components/account/OpenAIQuotaResetCell.vue`、`frontend/src/views/admin/AccountsView.vue`、`frontend/src/utils/openaiEndpointCapabilities.ts`。
- **测试**：`backend/internal/service/openai_codex_transform_test.go`、`backend/internal/service/openai_agent_identity_compat_test.go`、`backend/internal/service/openai_privacy_retry_test.go`、`backend/internal/util/httputil/httputil_test.go`、`backend/internal/handler/openai_alpha_search_test.go`、`backend/internal/service/openai_alpha_search_test.go`、`backend/internal/service/openai_account_scheduler_test.go`、`backend/internal/service/openai_quota_spark_window_test.go`、`backend/internal/service/openai_proxy_stream_circuit_test.go`、`backend/internal/service/openai_account_runtime_block_fastpath_test.go`、`backend/internal/service/openai_ws_v2/passthrough_relay_test.go`、`backend/internal/service/openai_ws_v2_passthrough_lifecycle_test.go`、`backend/internal/handler/openai_gateway_handler_test.go`、`frontend/src/components/account/__tests__/OpenAIQuotaResetCell.spark_shadow.spec.ts`、`frontend/src/views/admin/__tests__/AccountsView.batchTest.spec.ts`。
- **来源提交**：`2dcbd49c92b5affe47c6c7c423650271a50f8209`、`6ed8c0cfb516748d6bffa8a06b5a0586f6e4f3fc`、`16c1da45175d910ae03ca030933eba2286e37b20`、`fc56b7d78728b83fd4cd47dedddbbc055b34040b`、`fea2f5b59dd508df6838074962795d7fc3083a9e`、`83b22ecd2145efb46dae5a5e721f26e4c38a3031`、`e7e0d5a2cfd28940b7eb5f631eb3d7abaacaaf63`、`bfe241b37f5da9d7435506653acf731519718fa4`、`86b122c09c0595fa0cbf6d2cc813fe9a2cda0edf`。
- **上游部分吸收**：`30d2589ef0f0dc839b934b0b21a270d18b7af52b` 已在 ingress lease loss 时保留 terminal event；既有基线还原生包含 compact keepalive `response.failed`（`2f109e74c`）、Responses null tool schema 修复（`f3c94d209`）、pre-output capacity-shed failover（`c33c3208e`、`14a27f196`）、OAuth beta header 移除与 routing hints（`915cc7e7b`、`815035fcc`、`de349187d`）、调度阈值百分比保持（`99b31067f`）、个人订阅过期修正（`358e4a89a`）、response-model 审计（`db0bff82c`、`6e34fb09c`）、Chat reasoning alias（`8aa425d22`）、非法 reasoning item ID 清理（`9f31df3fa`）、空 `response.completed` failover（`280c1c862`）、可见输出 TTFT（`900194fab`）、HTML 403 账号中立分类（`12abb5470`）、OAuth 图片流错误 failover（`9763765eb`）和 Codex OAuth 设备指纹收敛（`c0ab3a00e`）。本次基线又吸收 remote compaction v2 与 native/legacy 路由分离（`9662cff2e`、`a8b9ea22b`，由 `1d3b9665c` 合入）、turn-state provenance/cross-account echo guard 与 session-level beta/native probe（`8219dcfc8`、`8ae6d8f67`），并将指纹收敛改为显式 opt-in 且覆盖 passthrough（`fce41e318`，三者由 `073e92d17` 合入）。这些上游行为不登记为 fork 差异；它们仍不覆盖 encrypted reasoning、Agent Identity 一次性恢复、外部取消 join、代理请求范围、health-neutral failover 与 quota 部分成功清理契约。
- **本次上游吸收**：`539064798`、`c3063e01a` 完成 request-scoped capacity recovery，`82cbe6aff` 允许 Responses WS 后续 turn 在尚未写出时对 429 failover，`b228b93e9` 修复 Chat 非流式缓冲读取错误的故障转移，`bfac49fef` 增加 Responses input-token 预检，`b94e484e2` 与 `5b2089c5a` 补齐 WS follow-up client-tool mapping 和 tool-search discovery output，`612436a5a`、`401dd43b4` 补齐 reasoning-content 回注。本条不再把这些子项作为 fork 差异；其余 encrypted reasoning、Agent Identity 一次性恢复、外部取消 join、代理请求范围、health-neutral failover 与 quota 部分成功清理契约继续保留。
- **本次基线继续吸收**：`cf3577a3c`、`acce29af2` 补齐 Responses 输入、tool schema、item ID、terminal usage/error 与 rejected-field retry，`25da02ddd` 避免 HTTP bridge 重放重复 tool call，`fa4587041` 增加 guardian parent affinity，`c374ff295` 统一网关故障转移与运维错误语义。它们不覆盖 fork 的实际分组归因、encrypted reasoning、外部取消 join、Agent Identity 一次性恢复、health-neutral failover 和 subscription 限额 429 契约。
- **本次基线新增吸收**：`f06bf181d` 提供 Responses/Chat/WS Fast `service_tier`，`6f972145b` 增加按用量阈值自动重置并复用共享 reset post-process，`243921dc0`、`31d5b67ba`、`7498d8fdc`、`7a09a2eaf`、`1563db3f8` 与 `e440ac48c` 完善 terminal output、工具别名、Responses Lite 和 rejected-field 兼容，`d493ce0bb`、`913ec5d74` 收敛 OAuth 账号身份与模型同步。它们不覆盖 fork 的实际分组归因、部分成功告警、外部取消 join、health-neutral failover 和 subscription 限额 429 契约。
- **本次基线继续吸收**：`d6012b0b3`、`53d76ad80`、`d5e43ef7d` 与 `095b52536` 补齐 Responses Lite 数字精度、tool-call mode、WS HTTP bridge 和 `parallel_tool_calls=false`，`8e60d5747` 保留 Codex `session-id`，`e55727d4c` 保持容量溢出时的 sticky binding，`e0e5e45cd` 保留恢复后的 tool-call item ID 类型。它们不覆盖 fork 的 encrypted reasoning、Agent Identity 一次性恢复、外部取消 join、代理请求范围、health-neutral failover 与 subscription 限额 429 契约。
- **本次基线继续吸收**：`22e1b8144` 至 `efb46db0a` 提供按实际路由生成的 Codex 模型目录、账号模型别名、稳定 capability 交集和 API Key 目录缓存隔离；`4795650d2` 记录错误请求的实际上游 endpoint，`d8694f03b` 覆盖 WSv2 陈旧 native tool ID 清理。这些上游行为不登记为 fork 差异；自动合并后仍保留 fork 的实际分组归因、encrypted reasoning、外部取消 join、health-neutral failover 与 subscription 限额 429 契约。
- **本次基线新增吸收**：`b7ec3cdad` 对无 terminal chunk 的 raw Chat 流返回故障，`81ac8ccd6` 让非流式 HTTP 200 terminal failure 进入账号 failover，`f4e3eb1c5` 在 WS v2 passthrough 中执行 Cyber policy，`c83dced4b` 将客户端正常关闭/断开排除出账号故障归因；这些上游行为不登记为 fork 差异，且不替代 fork 的实际分组、外部取消 join、health-neutral failover 与 subscription 限额 429 契约。
- **本次基线继续吸收**：`5d9c7abed` 将 Spark 配额 429 限定到具体模型，`3c22e78af` 保留 Spark quota reset 语义，`571d1e1d9` 隔离 WebSocket 语义限流，`804679d99` 让流式 failover 保留模型范围。Chat 与 WS HTTP bridge 的人工冲突解决保留 fork 的流式内存优化、client-tool 恢复和账号健康判定，同时把 `upstreamModel` / `mappedModel` 传入上游的模型级 failover；这些上游行为不登记为 fork 差异，也不覆盖 Ultrafast、实际分组、外部取消 join、health-neutral failover 与 subscription 限额 429 契约。
- **本次基线继续吸收**：`624e4eef6`、`8b4b3f4a9`、`2b8cb628b` 与 `d39fc491e` 将最终出站 `service_tier` 和上游回显分离，并在 Chat/Responses fallback 与 WebSocket 路径保留两者；`3a9070359`、`f323d8464`、`50ad6e2e5` 与 `82105f260` 让 Codex OAuth/setup-token 的 `default` 回显不再误降级 Fast 计费，同时保持公共 API 回显可权威降级并按实际 shadow 凭据判定。自动合并继续保留 fork 的 Ultrafast 原样转发、独立倍率和“响应只能降低、不能抬高计费 tier”契约。
- **人工合并解决**：`0b7eed0738a608971d9711e99ba824d89536f947` 保留 request-scoped proxy quarantine context；`caae38b9abf429d1326ec174b54210a21b023309` 保留 `ShouldUseOpenAIResponsesPassthrough` 和 compact-aware namespace 处理；`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 将 proxy quarantine、passthrough cancellation/close code 与上游 profit admission、load-shed 和 WS turn pricing 合并；`9527e0fc1d85897baf72fbb9ff32027ff3d63aaa` 合入上游取消检查与身份/配额恢复，并保留 public/fixed model、`ClientLifecycleContext`、`NeutralForAccountHealth` 和 `RequestScopedTransient`；`0bd492e7e7887cec0832981b27c4b164029a6c2c` 让上游 OAuth `count_tokens` HTML 403 fallback 复用 fork 已有的 privacy HTML 响应分类，消除同 package helper 重名且保留两边语义；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 OpenAI gateway、passthrough、WS 和调度冲突中保留协议恢复、取消与 health-neutral failover，并合入上游 response-model 审计、容量降载和 routing hints。上述提交中的 Project 归属和项目范围 scheduler cache 已失效，不得恢复。
- **前次合并解决**：合入上游父提交 `7b693ae4295e20329f18ff451b29a38879cb4705` 时，OpenAI HTTP/WS 继续保留 fork 的实际分组归因、passthrough 实际路径标记、reasoning cache、部分成功告警、外部取消和 health-neutral 语义，同时吸收上游 requested reasoning effort、Cyber passthrough 与客户端关闭归因；其中 Project 归属不再是当前契约。
- **合并审查**：逐项比较协议测试和状态清理，不得因为上游出现同名 helper 就删除本地行为；特别检查 streaming 已写出后的 failover、credential redaction 和 retry 次数。
- **删除条件**：上游逐项提供等价实现和测试后，可逐项缩减本条；全部不变量均等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test(AdminService_EnsureOpenAIPrivacy|TokenRefreshService_ensureOpenAIPrivacy|ForwardAlphaSearch|OpenAIProxyStreamCircuit|AccountTestServiceOpenAICompactAgentIdentity|OpenAIRuntimeBlock_|ApplyCodexOAuthTransform_)')
(cd backend && go test -tags=unit ./internal/handler -run '^(TestOpenAIGateway|TestAlphaSearch)')
(cd backend && go test ./internal/handler -run '^TestBillingErrorDetails_MapsSubscriptionUsageLimitsToTooManyRequests$')
(cd backend && go test ./internal/util/httputil -run '^TestIsCloudflareChallengeResponse$')
(cd backend && go test ./internal/service/openai_ws_v2 -run '^TestRelay_ContextCancellationJoinsBlockedDownstreamWrite$')
(cd backend && go test ./internal/service -run '^TestPassthroughLifecycle_LeaseLossSendsRetryClose$')
(cd backend && go test ./internal/service -run 'ServiceTier|Ultrafast' -count=1)
(cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run 'Test(ResetCreditLocalClearFailureReturnsConsumedResult|OpenAIGatewayService_ReportOpenAIAccountScheduleFailoverSkipsHealthNeutralFailures|.*ResetQuota)')
(cd frontend && ./node_modules/.bin/vitest run src/components/account/__tests__/OpenAIQuotaResetCell.spark_shadow.spec.ts src/views/admin/__tests__/AccountsView.batchTest.spec.ts)
```

## 8. Grok/xAI OAuth、协议与故障转移正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：修复 Grok OAuth 刷新与轮换凭据、Responses/Chat fallback、客户端工具往返和流式策略错误分类。
- **行为不变量**：refresh token rotation 不得丢失；OAuth 对账必须按账号身份匹配；协议不兼容时只回退可等价的 native Chat；client tool call/result 的 ID 与 payload 必须往返；流式内容策略错误不得误触发跨账号拼接。
- **当前代码**：`backend/internal/service/grok_oauth_reconciliation.go`、`backend/internal/service/oauth_refresh_api.go`、`backend/internal/service/openai_gateway_grok_chat_bridge.go`、`backend/internal/service/openai_gateway_grok_tool_protocol.go`、`backend/internal/service/grok_upstream_errors.go`、`backend/internal/pkg/apicompat/responses_client_tools.go`、`backend/internal/handler/admin/grok_oauth_handler.go`。
- **测试**：`backend/internal/service/grok_oauth_reconciliation_test.go`、`backend/internal/service/openai_gateway_grok_chat_bridge_test.go`、`backend/internal/service/openai_gateway_grok_tool_protocol_test.go`、`backend/internal/service/openai_gateway_response_flush_test.go`、`backend/internal/handler/admin/account_handler_grok_refresh_test.go`。
- **来源提交**：`971a0e5ca717f064afe72a750725f84d43d327fc`、`bce99892926c1e6d83201f74bb82ed74c6353afd`、`88c3138f1a717891eac3276c96eefd44237b45c3`、`e5944b9c7a0230edb0ebd577611c6d849bcb3d68`、`aee1f47c98b60af5fe630be84fc64d4b0e220d3a`。
- **上游部分吸收**：上游 Grok 完整集成由 `fb0475656` 合入，包含 `370bdcf69`、`25d2b03e9`、`e12e0dc1a`、`ec9e73360` 与 `74249b8fe` 等 OAuth、SSO、媒体和协议实现。本次基线进一步吸收 Grok 4.6（`a04ce4901`）、JWT tier 识别（`bb9e74285`）、分组长上下文控制（`fd82dfd52`）、媒体族 fallback（`e29b93a1f`）、原生及兼容 `x_search`（`0de6d7e9b`、`c4d883b8d`）和 usage guard（`5c52fa93d`、`8ea68bd68`）。本条只保留 refresh-token CAS 轮换、安全 Chat fallback、client-tool 往返和内容策略错误不 failover 四项仍有独立实现与回归的差异。
- **本次上游吸收**：`99a8b8470` 避免当前 turn 已含 inline image 时重复声明 `view_image`，`892787723` 保留 Grok 4.6 `xhigh` effort，`5b2089c5a` 补齐 tool-search discovery output；这些子项不再作为 fork 差异。
- **本次基线继续吸收**：`9ede0f716` 将 tool-search discovery 提升为可调用工具，`acce29af2`、`953028718`、`39aaf2fea` 与 `2e68b10aa` 补齐 Grok 兼容输入、容量重试、内容拒绝和媒体计费，`f7145c750` 将默认文本模型迁移到 Grok 4.6。fork 仍保留 refresh-token CAS、安全 Chat fallback、client-tool 往返和内容策略错误不跨账号 failover。
- **本次基线新增吸收**：`de6ef7134` 清理 Codex Responses 请求，`f4820c00d` 与 `fd872550d` 规范化非法 tool union root；这些协议清理来自上游，不登记为 fork 差异，也不替代 fork 的 OAuth 轮换、安全 Chat fallback 和 client-tool 往返契约。
- **人工合并解决**：`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 Grok OAuth、媒体和 OpenAI bridge 冲突中保留统一刷新入口、安全 fallback 与实际分组归因，并合入上游 SSO、密码授权和音视频一次性计费；其中 Project 校验已随项目空间移除而失效。
- **合并审查**：OAuth storage、admin refresh endpoint、scheduler token provider 和 protocol bridge 必须一起审查；只接受上游 UI 或单一 refresh helper 不构成吸收。
- **删除条件**：上游分别提供 OAuth 轮换、协议 fallback 和 client-tool 测试后逐项缩减，全部等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test(GrokOAuth|OpenAIGateway.*Grok|GrokUpstream)')
(cd backend && go test -tags=unit ./internal/handler/admin -run 'Test.*Grok')
```

## 9. 鉴权、会话绑定与审计状态正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：修复反向代理环境下 session binding 与安全元数据 IP 混用、超级管理员 TOTP 验证方式、多实例审计清理并发、邮箱别名去重与密码登录状态、fail-closed 验证码，以及 pending OAuth callback 的前端状态一致性。
- **行为不变量**：会话绑定使用稳定的 binding identity，审计日志仍记录真实安全元数据 IP；TOTP 必须通过用户配置的验证方式；多实例 clear 使用高水位和数据库状态 fencing，清理前已排队或并发写入的审计记录不得越过边界；邮箱别名候选超过单页上限时仍须完整查重，绑定真实邮箱后必须清除 `password_auth_disabled`；旧版或自定义 CSP 必须补齐 Aliyun CAPTCHA 的脚本和样式域名，否则不得启用会锁死鉴权入口的 fail-closed 配置；pending OAuth 发送验证码若已经进入 `choose_account_action_required`，所有 callback UI 必须立即消费该服务端状态，不得继续停留在失效的创建账号表单。
- **当前代码**：`backend/internal/pkg/ip/ip.go`、`backend/internal/server/middleware/session_binding.go`、`backend/internal/server/middleware/audit_log.go`、`backend/internal/server/middleware/security_headers.go`、`backend/internal/service/session_binding.go`、`backend/internal/service/totp_service.go`、`backend/internal/repository/user_repo.go`、`backend/internal/service/auth_email_binding.go`、`backend/internal/repository/audit_log_repo.go`、`backend/internal/service/audit_log_service.go`、`frontend/src/components/auth/PendingOAuthCreateAccountForm.vue`、`frontend/src/views/auth/OidcCallbackView.vue`、`frontend/src/views/auth/LinuxDoCallbackView.vue`、`frontend/src/views/auth/WechatCallbackView.vue`、`frontend/src/views/auth/DingTalkCallbackView.vue`。
- **迁移与测试**：`backend/migrations/182_audit_log_clear_state.sql`、`backend/internal/server/middleware/session_binding_test.go`、`backend/internal/server/middleware/security_headers_test.go`、`backend/internal/service/totp_verification_method_test.go`、`backend/internal/repository/user_repo_email_alias_test.go`、`backend/internal/service/auth_service_email_bind_test.go`、`backend/internal/repository/audit_log_repo_test.go`、`backend/internal/repository/audit_log_repo_sequence_integration_test.go`、`backend/internal/service/audit_log_service_test.go`、`backend/migrations/audit_log_clear_state_migration_test.go`、`frontend/src/components/auth/__tests__/PendingOAuthCreateAccountForm.spec.ts`、`frontend/src/views/auth/__tests__/OidcCallbackView.spec.ts`、`frontend/src/views/auth/__tests__/LinuxDoCallbackView.spec.ts`、`frontend/src/views/auth/__tests__/WechatCallbackView.spec.ts`。
- **来源提交**：`4d47d8916691de90d50c454a9935ef5f8a764994`、`8f10a05736dff37c6d275ef33f2fb6b3436ae3ef`、`fb693041dc6fbb4aff7dd4bbf0baa410e2a2ffa3`、`a106870c834e6cf021ffb5adea24c8f2d0ccb1dd`、`4ac9bb886564e861ed6668acd972e37798bd0aca`；密码登录状态恢复最初嵌入 merge `322ec3794da26e9db2aa5c39042a2769b32504b5`，属于既有历史结构缺口，本次不改写历史。
- **上游吸收**：`02e50cc22d038dabf3c6af92dbb92d1e0321f8d5` 已完整提供 pending-exchange 服务端账号接管防护及回归；`4ca86c52e` 已提供邮箱换绑别名检查与并发守卫，但其固定 50 条候选查询不覆盖 fork 的分页查重，且原子更新未处理 `password_auth_disabled`。fork 仍保留这些子项及创建账号表单立即发出 `complete` 并由各 callback 消费 choice state 的前端契约。
- **人工合并解决**：`8e34f01c53a650a00b80b7bf87476cb74f3118be` 在 CSP 冲突中合并上游腾讯验证码国内/国际区域来源，并保留 fork 的 Aliyun CAPTCHA 脚本与样式来源。
- **合并审查**：区分 trusted proxy 解析、binding key 和 audit IP；审计清理必须同时核对队列 drain、数据库锁顺序与持久化 clear watermark，不能用一个 `ClientIP` 字段重新承担全部语义；邮箱换绑接入新的原子 guard 时，仍须分页查完别名候选并恢复密码登录。
- **删除条件**：上游行为和并发/反代/TOTP 回归测试逐项等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/server/middleware -run 'Test.*SessionBinding')
(cd backend && go test -tags=unit ./internal/server/middleware -run 'Test(SecurityHeaders|EnhanceCSPPolicy)')
(cd backend && go test -tags=unit ./internal/service -run 'Test.*(SessionBinding|TOTP|AuditLogService)')
(cd backend && go test ./internal/repository ./internal/service -run 'Test(UserRepositoryExistsByEmailAlias|AuthServiceBindEmailIdentity_)')
(cd backend && go test ./internal/repository -run '^TestAuditLogRepository')
(cd backend && go test ./migrations -run '^TestMigration182AddsPersistentAuditClearState$')
(cd frontend && ./node_modules/.bin/vitest run src/components/auth/__tests__/PendingOAuthCreateAccountForm.spec.ts src/views/auth/__tests__/OidcCallbackView.spec.ts src/views/auth/__tests__/LinuxDoCallbackView.spec.ts src/views/auth/__tests__/WechatCallbackView.spec.ts)
```

## 10. 支付定价、结果 DTO 与退款状态机正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：分离余额充值倍率与订阅 CNY 定价，以 canonical USD/CNY rate 兼容旧配置；让公开支付结果不泄漏内部 DTO；退款回查以持久化 provider binding/snapshot 为准。
- **Ultrafast 计费不变量**：官方价格页尚无 Ultrafast 独立价格时，默认与 Fast 一致，按 Standard 的 `2 倍`计费；管理员可通过渠道模型定价的 `ultrafast_multiplier` 正数覆盖默认值，未配置时继续使用 2；请求 `ultrafast` 而上游响应回显 `priority` 或 `default` 时按实际较低档位降价，任何响应都不得把较低请求档位抬高到 Ultrafast；官方公布价格或可由真实账单可靠校准时，调整默认值而不改变协议透传和渠道覆盖能力。
- **Ultrafast 计费实现与测试**：代码位于 `backend/internal/service/billing_service.go`、`backend/internal/service/service_tier_billing.go`、`backend/internal/repository/channel_repo_pricing.go`、`backend/internal/handler/admin/channel_handler.go`、`backend/migrations/231_channel_ultrafast_multiplier.sql`、`backend/migrations/232_update_ultrafast_multiplier_comment.sql`、`frontend/src/components/admin/channel/PricingEntryCard.vue`；测试位于 `backend/internal/service/service_tier_billing_test.go`、`backend/internal/service/channel_pricing_multipliers_test.go`、`backend/migrations/ultrafast_multiplier_migration_test.go`、`frontend/src/components/admin/channel/__tests__/PricingEntryCard.timePricing.spec.ts`。提交后可运行 `git log -S'UltrafastMultiplier' -- backend/internal/service/billing_service.go` 定位当前变更。
- **行为不变量**：订阅金额只读取 `subscription_usd_to_cny_rate`，legacy multiplier 仅为派生兼容字段；显式 zero/disable 不得复活旧值；公开结果类型不得包含管理端字段；refund finalize 不得从当前订单猜 provider/refund ID，legacy Alipay audit 缺少精确渠道请求 ID 时必须人工核销，旧 pending audit 必须向后兼容，audit 查询失败或内容损坏必须 fail closed；provider 构建、merchant snapshot 校验和本地密钥预检必须先于退款 claim 与权益扣减，退款快照含商户身份或币种而 provider 不提供元数据时必须 fail closed，Wxpay 必须同时报告基础 `appId` 与 JSAPI `mpAppId` 并允许快照匹配实际下单模式使用的任一 AppID，provider 确定性业务失败必须返回 failed 而不能伪装成 pending，Alipay `20000` 或未知错误码等不确定响应必须保留 `REFUNDING` 并复用同一渠道请求 ID；扣减、`REFUNDING` claim 和带 UUID/开始时间的 `dispatching` attempt 快照必须同事务提交，force 余额扣减必须在事务内以 `refundAmount` 为上限按最新可用余额 clamp，不能受 prepare 旧值限制，非 force 余额扣减不足必须整体回滚并要求 force，即时 provider success 不得二次扣减，provider pending 后回查成功也仍须满足同一全额扣减约束；所有 provider outcome、补偿和终态转换必须在事务内校验当前 attempt ID 并重读最新 audit，旧 attempt 的延迟结果或锁前快照不得认领新重试、重复补偿或跳过重新扣减；本地 recovery lease 的 attempt ID 与渠道请求 ID 必须分离：同一次未知结果恢复只轮换 attempt ID 并保持渠道请求 ID，`REFUND_FAILED` 新重试必须生成新的渠道请求 ID；`REFUNDING` 恢复必须等待 5 分钟租约后优先查询 provider，查询与重放分别使用独立的 1 分钟和 3 分钟上下文，只有 Stripe、Alipay、Wxpay、Airwallex 可复用同一渠道请求 ID 重放，query capability 不得绕过该白名单，其余 provider 必须人工核销；provider 明确结果必须先把当前 attempt 原子推进到 `REFUND_PENDING` 并替换 audit，再执行成功或失败终态，未知结果则保留 `REFUNDING`；`REFUND_SUCCESS` 必须保留 attempt ID、渠道请求 ID、退款 ID 和扣减结果后才能删除 mutable pending audit，回查返回的非空退款 ID 必须在终态事务锁定并重读当前 attempt 后合并；legacy pending audit 缺少退款 ID 时，Wxpay 必须按本次退款折算后的渠道金额派生查询 ID，不得使用入账金额；成功和失败终态都必须先原子 claim `REFUND_PENDING`，旧失败查询不得覆盖已提交成功或触发二次退款，失败回查返回的新退款 ID 和 provider failure 必须在锁后重读 attempt，并在终态事务或补偿失败记录事务中重写 pending audit；即使补偿仍失败而保持 `REFUND_PENDING`，也必须在写入 `REFUND_ROLLBACK_FAILED` 的同一事务保留该新退款 ID，失败补偿成功后还必须写入 `deductionRollbackOK=true` 及 `balanceRolledBack`/`subDaysRolledBack`；`REFUND_PENDING` 只能查询或人工核销，不得重新进入 provider refund；`REFUND_FAILED` 重试必须保持原金额、原因、force 和扣减意图，准备阶段必须存在 pending audit 并记录当时订单状态与上一 attempt ID，claim 必须在同一事务内同时匹配该状态和 attempt generation，缺失审计或代际变化必须 fail closed，未补偿的 `REFUND_ROLLBACK_FAILED` 必须在 provider 调用前阻断，补偿成功时必须在同一事务标记 `resolved`；审计表本次先以 expand migration 新增退款状态 `(order_id, action)`、同订单返利 APPLIED/SKIPPED claim 和同订单 `SUBSCRIPTION_ASSIGNED` 三项部分唯一约束，同时保留历史 `(order_id, action)` 全局唯一索引供旧二进制继续匹配无谓词 `ON CONFLICT`；只有下一 fork release 确认旧实例全部下线后，才可用独立 contract migration 删除全局索引并启用普通审计 action 重复追加；进入 pending 的订单状态与当前退款 ID/扣减明细必须通过匹配退款部分唯一索引的原生 upsert 在同一事务内原子替换；余额补偿只能调整 `balance`，不得增加 `total_recharged`；订阅退款必须区分“缩短”与“全量扣减”并可精确补偿，全量扣减必须保留同一订阅行、置为过期并把扣减前后期限写入 audit，失败补偿须在该行锁内合并期间发生的续期，legacy soft-deleted audit 仍可恢复；退款扣减、续期、补偿和兑换码负向调整必须在同一事务内锁定订阅行后基于最新 `expires_at` 重算；幂等分配在锁后发现订阅已被并发请求续期或暂停时，不得再次延长或重新激活；外层事务内不得提前失效订阅缓存，只有 commit 成功后才能同步清除本机 L1、分布式缓存并发布跨实例失效；分布式删除与跨实例发布必须分别使用独立有界上下文并全部尝试，错误合并上报，且缓存失效错误不得记作订阅扣减或补偿失败。
- **当前代码**：`backend/ent/schema/payment_audit_log.go`、`backend/migrations/194_payment_audit_action_idempotency_scopes.sql`、`backend/internal/repository/user_repo.go`、`backend/internal/repository/user_subscription_repo.go`、`backend/internal/service/payment_amounts.go`、`backend/internal/service/payment_config_service.go`、`backend/internal/service/payment_order.go`、`backend/internal/service/payment_order_provider_snapshot.go`、`backend/internal/service/payment_refund.go`、`backend/internal/service/subscription_service.go`、`backend/internal/service/redeem_service.go`、`backend/internal/payment/provider/stripe.go`、`backend/internal/payment/provider/alipay.go`、`backend/internal/payment/provider/wxpay.go`、`backend/internal/payment/provider/airwallex.go`、`backend/internal/handler/payment_handler.go`、`frontend/src/views/admin/SettingsView.vue`、`frontend/src/views/admin/orders/AdminOrdersView.vue`、`frontend/src/views/user/PaymentView.vue`。
- **测试**：`backend/internal/service/payment_config_service_test.go`、`backend/internal/service/payment_order_result_test.go`、`backend/internal/service/payment_order_provider_snapshot_test.go`、`backend/internal/service/payment_refund_test.go`、`backend/internal/service/payment_refund_integration_test.go`、`backend/internal/service/subscription_renewal_lock_test.go`、`backend/internal/service/subscription_revoke_cache_test.go`、`backend/internal/repository/user_subscription_lock_test.go`、`backend/internal/repository/user_repo_integration_test.go`、`backend/internal/repository/user_subscription_repo_integration_test.go`、`backend/internal/payment/provider/stripe_test.go`、`backend/internal/payment/provider/alipay_test.go`、`backend/internal/payment/provider/wxpay_test.go`、`frontend/src/views/admin/__tests__/SettingsView.spec.ts`、`frontend/src/views/user/__tests__/PaymentView.spec.ts`。
- **来源提交**：`cdc7fa66b303333c00b87d5d0852a6d4af9993b7`、`d6f78bf2dc6c11b0c5fe72c4a8f2c92eaf1b7425`、`b5af02ae64ae12254add841f555fbf9538d9eeef`、`427c983f92b1dd7e3815e50bdbdc32caf641cf57`、`51775c230a7575ae0ca70dab751cece53163d35d`。
- **上游相邻行为**：`99b357083e1b5a860f9987523434092e5ef2fcfa` 已原生提供 daily-midnight reset，并将 daily 与 weekly/monthly 使用不同窗口锚点；本次基线又以 `9096492b5`、`b689e5b40`、`e5b325e48` 和 `9261dd773` 完善 response-model 与 service-tier 计费。它们不覆盖 canonical CNY rate、公开 DTO、provider snapshot 和退款状态机；合并时仍须同时保留 fork 的行锁和 commit 后缓存失效。
- **本次基线新增吸收**：`6466978d2` 统一 token 计费路径并提供上下文阶梯价，`75faedda9` 让 Fast/Priority 按上游实际档位只降不升计费，`695ebede7` 规范化 CN Anthropic usage token。它们不改变 fork 的 canonical CNY rate、公开 DTO、provider snapshot 与退款状态机边界。
- **当前修复定位**：提交后运行 `git log -S'recoverRefundingAttempt' -- backend/internal/service/payment_refund.go`、`git log -S'phase\": \"dispatching' -- backend/internal/service/payment_refund.go` 与 `git log -S'MerchantIdentityMetadata' -- backend/internal/payment/provider/wxpay.go`。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309` 在 `payment_config_service.go` 及测试中保留 canonical rate、legacy 派生和输入校验；`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 将上游事务化 pending-refund finalize 与 fork 的 provider snapshot、退款 ID 和旧 pending audit 兼容语义合并；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在订阅仓储冲突中保留行锁与缓存失效契约，并合入 daily 与 weekly/monthly 分离的时间锚点；其中 Project 过滤已失效。
- **合并审查**：配置读写、下单展示、provider snapshot、退款 audit 和公开 DTO 必须一起比较；只吸收一个字段名不构成等价。
- **删除条件**：上游分别提供 canonical rate、公开 DTO 边界和 snapshot refund 回归测试后逐项缩减，全部等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test(ParsePaymentConfig|.*PaymentConfig|.*Refund)')
(cd backend && go test -tags=unit ./internal/service ./internal/payment/provider -run 'Test(WxpayMerchantIdentityMetadataIncludesBothAppIDs|ValidateRefundProviderSnapshotMetadataFailsClosedWithoutIdentity|ValidateRefundProviderSnapshotMetadataAcceptsWxpayJSAPIAppID|ExecuteRefundRejectsWxpayMerchantMismatchBeforeClaimAndDeduction)$')
(cd backend && go test -tags=unit ./internal/service -run 'Test(FinalizeRefundFailedRejectsStaleCallerAfterSuccess|ExecuteRefundImmediateSuccessDeductsAvailableBalanceOnce|RefundFinalizePlanUsesOrderForceForLegacyAudit)$')
(cd backend && go test -tags=unit ./internal/service ./internal/payment/provider -run 'Test(PrepareRefundRejectsPendingOrder|ExecuteRefundRejectsPendingOrderBeforeProviderCall|RefundingAttemptRecoversFromProviderOutcomePersistenceFailure|RefundingAttemptRequiresManualReconciliationForUnsafeProvider|RefundingAttemptDoesNotReplayUnsafeQueryProvider|RefundingAttemptUsesFreshContextForReplay|QueryAndFinalizeRefundRequiresManualReconciliationForLegacyAlipayAudit|QueryAndFinalizeRefundForceUsesLatestAvailableBalance|QueryAndFinalizeRefundFailedPersistsObservedRefundID|QueryAndFinalizeRefundPreservesLegacyWxpayRefundID|AlipayRefundEndpointsRequirePersistedProviderRequestID|AlipayRefundReturnsFailedForBusinessFailure|AlipayRefundKeepsIndeterminateFailureUnresolved|AlipayRefundRejectsNilResponse|AlipayQueryRefundUsesPersistedProviderRequestID)$')
(cd backend && go test -tags=unit ./internal/service -run 'Test(RefundRetryBlockedWhileRollbackRequiresReconciliation|RefundRetryAllowedAfterRollbackFailureIsResolved|RefundFailedWithoutPendingAuditRequiresReconciliation|RefundAttemptClaimFencesPreparedStatusAndFailedGeneration|QueryAndFinalizeRefundRetriesRollbackBeforeMarkingFailed|QueryAndFinalizeRefundKeepsPendingWhenFailedRollbackStillFails|QueryAndFinalizeRefundFailsClosedOnPendingAuditQueryError|QueryAndFinalizeRefundFailsClosedOnCorruptPendingAudit|RefundRetryPreservesParametersAndReplacesPendingAudit|RedeemSubscriptionInvalidatesReloadedL1AfterCommit)$')
(cd backend && go test -race -tags=unit ./internal/service -run 'Test(ExecuteRefundRequiresForceWhenConcurrentSpendLeavesShortDeduction|ExecuteRefundValidatesProviderBeforeClaimAndDeduction|DelayedRefundOutcomeCannotClaimNewAttempt|RefundingAttemptRecoversFromProviderOutcomePersistenceFailure)$')
(cd backend && go test -tags=integration ./internal/service -run 'Test(RefundAuditUpsertSurvivesConcurrentPostgresConflict|PaymentAuditIdempotencyScopesOnPostgres|PaymentAuditExpandMigrationDeduplicatesAffiliateClaims|RefundFinalizationUsesLatestAuditAfterPostgresLock|FullRefundDeductionCompensationMergesConcurrentRenewalOnPostgres)$')
(cd backend && go test -tags=integration ./internal/repository -run '^TestUserRepoSuite/TestRefundDeductionRollbackLeavesRechargeTotalUnchanged$')
(cd backend && go test -tags=integration ./internal/repository -run '^TestSubscriptionExpiryAdjustmentsSerializeOnPostgres$')
(cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/SettingsView.spec.ts src/views/user/__tests__/PaymentView.spec.ts)
```

## 11. 用量与运维可观测性、查询性能

- **生命周期**：`等待上游吸收`
- **原始意图**：避免大表分页重复精确 COUNT，统一全局筛选口径，暴露 usage-record runtime，恢复已删除 API Key 归因，并保持 Dashboard 健康因子的统计语义一致。
- **行为不变量**：翻页复用同一筛选条件的精确总数，筛选变化必须失效缓存；列表、统计和错误页使用同一全局 filter scope；deleted key 只用 digest 归因；健康分不得混用语义不同的指标；旧 `project_id` 仅作为兼容存储，不得参与范围过滤或聚合分组；IP geo 仅显式启用。
- **当前代码**：`backend/internal/service/ops_log_runtime.go`、`backend/internal/service/ops_dashboard.go`、`backend/internal/service/ops_health_score.go`、`backend/internal/repository/ops_repo.go`、`backend/internal/repository/usage_log_repo.go`、`backend/internal/repository/usage_log_repo_trend.go`、`frontend/src/views/admin/UsageView.vue`、`frontend/src/views/admin/ops/OpsDashboard.vue`、`frontend/src/components/admin/usage/UsageTable.vue`、`frontend/src/components/common/IpGeoCell.vue`、`frontend/src/utils/ipGeoLookup.ts`。
- **迁移与测试**：`backend/migrations/185_deleted_api_key_audit_digest.sql`、`backend/internal/service/ops_dashboard_test.go`、`backend/internal/service/ops_log_runtime_test.go`、`backend/internal/repository/ops_deleted_key_audit_test.go`、`backend/internal/repository/usage_log_repo_integration_test.go`、`frontend/src/views/admin/__tests__/UsageView.spec.ts`、`frontend/src/views/admin/ops/__tests__/OpsDashboard.operator.spec.ts`、`frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`、`frontend/src/components/common/__tests__/IpGeoCell.spec.ts`、`frontend/src/utils/__tests__/ipGeoLookup.spec.ts`。
- **来源提交**：`2f78dfc6a00a7cc6de285da6ceabe9868c58a4d7`、`7a17a98b56b7747748f40587cef377bc82143eb2`、`4ec3ddb89297fc8960a8acd9546f72e14bc218f4`、`4cb1350172558b987651097212b22234568dcf79`、`763a01225ffaaf5acd890c765d62a18129f4cee1`、`60cfa88334299f1a471a6eb146668bd46a521e30`、`df0f38f62de8430eb3780bb64716fc645fe2ebc0`、`440386f5ff1ba22c2911e7392177025ef4a42d58`。
- **上游部分吸收**：Channel Monitor V2、upstream response-model 审计（`db0bff82c`、`6e34fb09c`）和系统日志落库退避（`e687ca3e9`）均来自当前上游基线；`c204d33b0` 又合入分组每日 rollup、时区迁移、trigger 驱动重算和今日/昨日/总用量 API/UI（核心实现 `cb7b03795`，测试时区隔离修复 `45dcce0e4`）。这些上游行为不登记为 fork 差异；本条只保留精确 COUNT 缓存、deleted-key digest 归因、共享全局筛选口径和 opt-in IP geo。
- **本次上游吸收**：`a9514a68d` 将总计、入口、上游入口和路径统计合并为一次 `GROUPING SETS` 扫描；本次合并让该查询继续复用 fork 的 `buildUsageLogFilterWhere`，共享筛选口径不变。
- **本次基线继续吸收**：`c374ff295`、`e4f869e0c` 与 `6b0ec50f2` 完善运维错误捕获、详情兼容展示和 SLA 排除语义；本次合并继续保留 fork 的 deleted-key digest 与隐私约束。
- **本次基线新增吸收**：`cfecc8d11` 让错误详情返回列表时保留筛选状态，`cd05772e9` 避免混用 cgroup 与宿主机内存指标；它们不覆盖 fork 的共享筛选口径和 deleted-key digest 归因。
- **本次基线新增吸收**：`11ada80d5`、`5705f4a4a` 与 `a8cfe746b` 记录并展示策略映射前的 requested reasoning effort，同时只向管理员暴露映射后值；该上游审计能力不替代 fork 的入口/上游 endpoint、response-model 审计和共享查询口径。
- **当前修复定位**：提交后运行 `git log -S'buildUsageLogFilterWhere' -- backend/internal/repository/usage_log_repo.go`。
- **人工合并解决**：`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 dashboard、usage query/cache 和写入冲突中保留共享筛选与精确 COUNT 缓存，同时合入 request type、upstream response-model 和 mismatch 审计。旧 `project_id` 写入现在只用于存储兼容，Project scope 不再是当前契约。
- **合并审查**：同时核对 query、count、dashboard、error tabs 和权限过滤；任何基于旧 `project_id` 的范围过滤或分组都应移除。
- **删除条件**：上游提供等价筛选、COUNT 缓存和 deleted-key attribution 测试后逐项缩减。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service ./internal/repository -run '^TestOps')
(cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/UsageView.spec.ts src/views/admin/ops/__tests__/OpsDashboard.operator.spec.ts src/components/admin/usage/__tests__/UsageTable.spec.ts src/components/common/__tests__/IpGeoCell.spec.ts src/utils/__tests__/ipGeoLookup.spec.ts)
```

## 12. 管理端交互与紧凑诊断体验

- **生命周期**：`等待上游吸收`
- **原始意图**：让账号错误在主列表保持短码、详情进入 tooltip；Pagination 尊重传入的大页选项；DataTable 各渲染路径统一忽略交互控件 row-click；iOS 输入聚焦不触发页面缩放。
- **行为不变量**：主表不得铺开完整 JSON/error message；`status` 裸字段不得误识别为错误码；500/1000 page size 不得被默认配置覆盖；按钮、链接、输入框和 `data-row-click-stop` 不得触发行点击；iOS 修复保持 CSS/viewport 可访问性。
- **当前代码**：`frontend/src/components/account/AccountStatusIndicator.vue`、`frontend/src/components/common/Pagination.vue`、`frontend/src/components/common/DataTable.vue`、`frontend/src/style.css`。
- **测试**：`frontend/src/components/account/__tests__/AccountStatusIndicator.spec.ts`、`frontend/src/components/common/__tests__/Pagination.spec.ts`、`frontend/src/components/common/__tests__/DataTable.spec.ts`、`frontend/src/__tests__/iosInputZoom.spec.ts`。
- **来源提交**：`66601d69641ed0ae3bf665721c7ea5aaf8413be3`、`470c597201dbb00e2b68060f98f19c240f4f33ea`、`0407629d24dd5fd312d39a4f37633134b8702e75`、`5b9e2eb6948544409b0304f080b211b2522369c6`。
- **当前修复定位**：提交后运行 `git log -S'roleLabel' -- frontend/src/components/layout/AppHeader.vue` 与 `git log -S"wrapper.emitted('account-state-reset')" -- frontend/src/components/account/__tests__/AccountUsageCell.spec.ts`。
- **上游相邻行为**：当前上游已扩展 Grok temporary-unschedulable 状态、账号优先级、无限并发输入和 Plugins 超级管理员入口；它不覆盖短错误码/完整 tooltip、Pagination、DataTable row-click 和 iOS 输入缩放契约，因此本条继续保留。
- **人工合并解决**：无相关人工解决锚点。
- **合并审查**：以交互和布局契约为准，不以组件是否仍存在同名 class/prop 为准；桌面表格、移动卡片和虚拟列表都要覆盖。
- **删除条件**：上游分别提供等价交互及回归测试后逐项缩减。
- **聚焦验证**：

```bash
(cd frontend && ./node_modules/.bin/vitest run src/components/account/__tests__/AccountStatusIndicator.spec.ts src/components/common/__tests__/Pagination.spec.ts src/components/common/__tests__/DataTable.spec.ts src/__tests__/iosInputZoom.spec.ts)
```

## 13. 相对部署、示例配置与迁移恢复

- **生命周期**：`等待上游吸收`
- **原始意图**：保证 `/sub2api/api/v1` 等相对 API base 可生成绝对 gateway URL，示例 YAML 可被真实配置加载器解析，并让 fork migration `177_account_duplicate_operation_index_notx.sql` 中断后可清理 invalid index 再重试。
- **行为不变量**：浏览器和无 `window` 环境都不得把相对 base 错拼进 Axios；示例 YAML 必须持续通过配置解析；migration `177` 重试不得被上一次 invalid index 永久阻塞。首次 super-admin 初始化不再创建 Project owner membership。
- **当前代码**：`frontend/src/api/url.ts`、`deploy/config.example.yaml`、`backend/internal/repository/migrations_runner.go`、`backend/internal/setup/setup.go`。
- **测试**：`frontend/src/api/__tests__/url.spec.ts`、`backend/internal/config/config_test.go`、`backend/internal/repository/migrations_runner_notx_test.go`、`backend/internal/setup/setup_test.go`。
- **来源提交**：`38d6294088369255e79463d836515f7df527f271`、`b255b687ba7872ad7bc590a88a85919cc0253ec9`、`0306521a5557a2520a54c32a014ee7831929397f`。
- **上游相邻行为**：`db0bff82c7f954607f4b66421a79200e150ac836` 为上游 migration `195_add_usage_log_upstream_model_mismatch_index_notx.sql` 增加了同类 invalid-index recovery；本次基线又以 `6a1efda0c`、`e2263d256`、`10081a812` 保留 compose gateway 默认配置并转发 force HTTP 设置。它们不等于上游已吸收 fork 的相对 URL或 migration `177` 契约。
- **人工合并解决**：`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 non-transactional migration runner 冲突中同时保留 fork migration `177` 与上游 migration `195` 的 invalid-index 恢复。
- **合并审查**：相对 URL、示例配置和 migration runner 分别比较；上游只修复其中一项时只删除对应子项。
- **删除条件**：相对 URL、示例配置和 migration `177` 恢复分别有等价上游实现与测试后逐项缩减，全部吸收后删除。
- **聚焦验证**：

```bash
(cd frontend && ./node_modules/.bin/vitest run src/api/__tests__/url.spec.ts)
(cd backend && go test ./internal/config -run '^TestExampleConfigIsValidYAML$')
(cd backend && go test ./internal/repository -run '^TestApplyMigrationsFS_NonTransactionalMigration')
```

## 上游合并检查清单

1. 用当前分支 first-parent 历史重新定位最新上游 merge，并记录 merge SHA、两个父提交及新的上游父提交；不要直接把移动的 `upstream/main` 写成基线。
2. 分开审查 `长期保留` 与 `等待上游吸收`：前者检查不变量是否仍成立，后者逐项寻找上游等价实现和测试。
3. 对所有有人工冲突解决的 merge 运行 `git show --remerge-diff <merge-sha>`；只有非空且与条目相关的解决才作为锚点保留。
4. schema、migration、Wire provider 或生成入口变化后，从 source 解决冲突并运行 `(cd backend && make generate)`，不要手改生成输出作为最终解决。
5. 运行条目内聚焦验证，再按改动范围运行仓库 gate：

```bash
(cd backend && make test-unit)
(cd backend && make test-integration)
(cd backend && golangci-lint run ./... --timeout=30m)
make test-frontend
git diff --check
```

6. 确定合并后的 fork 版本：上游版本变化时使用 `<upstream-version>-fork.1`，否则递增同一 base 已发布的最高 revision。
7. 重新检查 `baseline..HEAD` 的净行为，删除已经被上游等价吸收且有回归覆盖的条目。
8. 代码、测试和 `FORK.md` 必须在同一提交同步；公共能力还要同步全部维护语言的用户文档和当前 changelog。

## 明确排除

- **Merge 历史**：纯上游 merge 和已被后续实现替代的人工解决不作为独立能力；当前只引用 `a8a3c18641fb1c00030c2baa22fc3918c9e44e68`、`0bd492e7e7887cec0832981b27c4b164029a6c2c`、`8e34f01c53a650a00b80b7bf87476cb74f3118be`、`9527e0fc1d85897baf72fbb9ff32027ff3d63aaa`、`d585df8d934807b5eaa3d65aac8cbb2954fa1519`、`caae38b9abf429d1326ec174b54210a21b023309` 与 `0b7eed0738a608971d9711e99ba824d89536f947` 中仍与能力不变量相关的解决。
- **已替代方案**：`6b37423465f1780dda62232d88e53676c4af15d5` 与 `bb9e60b13bab1a7f7c31d8987dfa00d5ed6da8ef` 的旧 operator/group-scope 方案，以及后续 Project 空间模型，均已由能力 1 的全局管理员权限替代，不单列。
- **派生输出**：单纯 VERSION 同步、Ent/Wire 生成输出和 locale 补齐不单列；它们归属于对应 source capability。
- **内容与机械变更**：赞助商文案、链接修正、格式、lint annotation 和仅测试适配不单列。
- **迁移文件编号**：fork 的 `231_channel_ultrafast_multiplier.sql` 与上游的 `231_add_usage_log_requested_reasoning_effort.sql`、`231_user_restrict_public_groups.sql` 可以并存；migration runner 以完整 filename 作为 `schema_migrations` 主键并校验各自 checksum，三个迁移均幂等且互不依赖，不为消除数字前缀重复而改名。
- **已合并上游修复**：`21aacde0b3d340e21253b73a04f6e724b40a77de` 已通过上游父提交 `b74024c7868ee88a0bf921306cbc22a2f922872a` 进入当前基线；其让下行 write context 脱离 relay cancellation 的基础修复不是 fork 差异。当前 fork 额外保证外部取消会关闭连接并 join 阻塞写，该行为归入能力 7，不单列新能力。
- **既有上游吸收**：上游父提交 `27e8f69a9e04d5919c7f4b6a4175c34af24e7eb2` 已提供 Stripe 金额级幂等键、pending refund 的事务化 claim/finalize、可用余额原子扣减，以及 Messages 临时账号错误切换；这些上游子能力不作为 fork 差异。能力 7 与 10 只保留仍超出上游的协议、provider snapshot、退款审计兼容等不变量。
- **既有上游部分吸收**：上游父提交 `00b8596176809906993169c283671811ad04f58d` 包含 `1b04e03cc4c7c23c216ae0f4830b593700b06eda` 的 Responses `output_text` 解析和 `30d2589ef0f0dc839b934b0b21a270d18b7af52b` 的 lease-loss terminal event 保留；能力 5 与 7 只移除这些重叠子项，其余隐私、授权、fail-closed、取消、代理、故障转移和配额清理契约继续保留。
- **既有基线上游新增**：Composite 分组模型广场、Codex WebSocket prewarm continuation、OAuth `count_tokens` HTML 403 fallback、Grok 视频 `task_id`、Gemini 3.6 Flash 模型、Ops 自定义错误时间范围，以及 upstream transport / SOCKS5 的 TCP 建连超时均来自既有上游基线，不登记为 fork 能力；`count_tokens` 人工解决仅复用 fork 已有的 privacy HTML 响应分类。
- **当前基线上游原生能力**：Composite 图片/Codex/CN/视频/Messages 路由、OAuth outbound plugin、daily-midnight/阈值自动 reset、Channel Monitor V2、response-model/service-tier/Fast 与渠道时段/阶梯计费、service-tier 请求/响应分离与 Codex OAuth 计费判定、requested reasoning effort、系统日志退避、Grok 4.6/JWT tier/x_search/inline-image/retry/Codex 请求清理、Codex OAuth 指纹和账号身份收敛、分组每日 rollup、用户公开分组限制、remote compaction v2/turn-state provenance、native compaction v2 用量记录与筛选、request-scoped capacity recovery、Responses WS session preemption/后续 turn 429 failover/Cyber policy/client-close attribution、passthrough WebSocket session 隔离、oversized passthrough HTTP bridge、client-tool discovery/follow-up、HTTP bridge replay 去重、adaptive protocol、guardian parent affinity、用量单次聚合、模型广场、按实际路由生成的 Codex 模型目录、实际上游 endpoint 错误观测、WSv2 陈旧 native tool ID 清理、Plugins 管理入口、Go 1.27.0 builder、Claude Code Messages 粘性路由、Responses 透传首输出前 keepalive、Grok cache key 优先级与 vision tool output 图片保留、Anthropic 工具参数保真、Antigravity 混合内置工具、周/月订阅重置锚点、分组局部更新保留未提交限额、配额 cooldown 原子重置与 scheduler rate-limit 重置、配额 singleflight 去重、可配置图片工具 cooldown、Ollama Cloud 国产平台用量、OpenAI refresh token 重新授权、兑换码本地时区过期解析、智谱团队 Coding Plan、轻量倍率快照刷新、批量关闭指纹收敛、Claude attribution header 保留、充值币种展示、连字符版本后缀解析、Spark 模型级限流与重置语义，以及 WS/流式模型级 failover 均已包含在 `fdf9751c1223a74a7153e537c6d9d1fb14ee9cad` 基线，不登记为 fork 能力，也不得在后续冲突中因同名本地 helper 而删除。
