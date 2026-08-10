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
| 最新已合并上游 merge | `a8a3c18641fb1c00030c2baa22fc3918c9e44e68` |
| Fork 父提交 | `2edd0eb55ba24a0c9d2d52ccc93ae552062f14e1` |
| 上游父提交 / 比较基线 | `10a4c6e3ad319587e817109c071259269855ec30` |
| 当前比较范围 | `10a4c6e3ad319587e817109c071259269855ec30..HEAD` |

`upstream/main` 是移动目标，不自动等于本文档基线。本次合并固定候选为 `10a4c6e3ad319587e817109c071259269855ec30`；远端后续推进不改变本次 merge 的第二父，下一次审计仍须从最新已合并上游 merge 重新确定基线。

## Fork 发布版本

| 项目 | 值 |
| --- | --- |
| 权威上游版本源 | 上游父提交中的 `backend/cmd/server/VERSION` |
| Fork 版本源 | `backend/cmd/server/VERSION` |
| 当前上游版本 | `0.1.173` |
| 当前 Fork 版本 | `0.1.173-fork.1` |
| 已发布同基线版本 | 无 |
| 下次发布所需版本 | `0.1.173-fork.1`（本次合并已同步版本源） |

所有 fork release 必须使用 `<upstream-version>-fork.<N>`。上游版本变化时从 `fork.1` 重新开始；同一上游版本的后续 fork release 从已发布的最高 `N` 递增，不得以 plain upstream version 发布 fork 构建。

## 能力索引

| # | 能力 | 生命周期 |
| --- | --- | --- |
| 1 | 项目空间与资源权限隔离 | `长期保留` |
| 2 | 分组订阅 5 小时额度与实际调度归因 | `长期保留` |
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

## 1. 项目空间与资源权限隔离

- **生命周期**：`长期保留`
- **原始意图**：用 Project 成员、应用配置和资源绑定替代旧 operator-role/分组权限边界，使账号、分组、代理、API Key、用量、日志、批量图片任务和管理操作可按项目隔离。
- **行为不变量**：外部调用使用 API Key 所属项目；项目管理员不得跨项目读取或修改资源，也不得通过通用账号更新修改仅超级管理员可控的上游计费探测或倍率同步设置；Project Profile 的 restricted/unrestricted 模式、绑定和激活状态必须一致；资源移动后日志归属、缓存和调度快照必须同步；shadow/linked account 必须继承父账号项目。
- **当前代码**：`backend/ent/schema/project.go`、`backend/ent/schema/project_member.go`、`backend/ent/schema/project_profile.go`、`backend/ent/schema/project_profile_binding.go`、`backend/internal/repository/project_context.go`、`backend/internal/repository/project_repo.go`、`backend/internal/service/project_context.go`、`backend/internal/service/project_service.go`、`backend/internal/service/admin_account.go`、`backend/internal/handler/admin/access_scope.go`、`backend/internal/handler/admin/project_handler.go`、`backend/internal/handler/admin/account_handler.go`、`frontend/src/api/admin/projects.ts`、`frontend/src/views/admin/ProjectsView.vue`。
- **迁移与测试**：`backend/migrations/154_project_isolation_default_project.sql` 至 `backend/migrations/159_project_scoped_proxies.sql`、`backend/migrations/170_batch_image_project_scope.sql`、`backend/internal/repository/project_context_test.go`、`backend/internal/service/project_service_test.go`、`backend/internal/service/admin_account_upstream_billing_probe_test.go`、`backend/internal/handler/admin/operator_account_scope_test.go`、`backend/internal/server/routes/admin_permission_routes_test.go`、`frontend/src/views/admin/__tests__/ProjectsView.spec.ts`。
- **来源提交**：`0a69e2c6055e7b1b0b9d861c8b4c787f70fbc107`、`98d869de85ff67cd53477c6a8736d593f64337c7`、`e5340bf36cee7123da34599ca346647fd1b0a2a9`、`c59d0a28f3e7c930872dd804ef8d0243a4bdd06e`、`65c3150e89032d66e7fceafe7dc316bc9bdc0a60`、`f4f57f0f97c8898c78869ec1b032bab36190e689`。
- **当前修复定位**：提交后运行 `git log -S'input.ProbeEnabled, input.RateSyncEnabled' -- backend/internal/service/admin_account.go`。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309` 在 `backend/cmd/server/wire_gen.go` 中保留 Auth/Passkey 对 `projectService` 的注入；`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 在账号仓储和管理服务冲突中继续保留 Project scope 与 shadow 父项目继承；`9527e0fc1d85897baf72fbb9ff32027ff3d63aaa` 在 OAuth quota 路由、handler、Wire 与 scheduler snapshot 中同时保留项目权限/可见性、项目范围缓存隔离，并合入上游 quota refresh/recovery；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在权限、鉴权缓存、账号管理和用量冲突中保留 Project scope，同时合入上游 Grok 媒体与 response-model 审计字段。
- **合并审查**：搜索新增的 admin route、repository query、cache key、scheduler snapshot 和后台原始 `fetch`；确认都显式继承 Project 上下文，不得只依靠前端隐藏入口。
- **删除条件**：不主动删除。只有维护者明确放弃项目空间产品能力时，才可按独立迁移方案移除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run '^TestProjectService')
(cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run 'TestProjectAdmin.*UpstreamBillingProbe')
(cd backend && go test -tags=unit ./internal/repository -run 'Test(Project|.*ProjectScope|.*ProjectProfile)')
(cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/ProjectsView.spec.ts)
```

## 2. 分组订阅 5 小时额度与实际调度归因

- **生命周期**：`长期保留`
- **原始意图**：为 subscription group 提供用户级 5 小时 USD 窗口，并确保 fallback/composite 调度后的限额、计费、并发、sticky session 和 Cyber policy 归因符合各自契约。
- **行为不变量**：API Key 自身限流与分组 5 小时窗口保持两条独立逻辑；普通分组不得误用 subscription 窗口；真实 fallback 后的检查、利润准入和记账必须使用 `AccountSelectionResult` 的有效分组，不得回退到原 `apiKey.Group`；composite 只路由目标平台、不携带成员分组关系，因此 selection 与 usage 继续归属于 API Key 所属父分组，且 composite 本身不得凭空安装成员分组利润门；同一请求共享一个 `PricingAt`。
- **当前代码**：`backend/ent/schema/user_group_rate_limit_window.go`、`backend/internal/repository/user_group_rate_limit_window_repo.go`、`backend/internal/service/user_group_rate_limit_window_port.go`、`backend/internal/service/gateway_request_pricing.go`、`backend/internal/service/gateway_profit_control.go`、`backend/internal/service/openai_profit_control.go`、`backend/internal/handler/gateway_handler.go`、`backend/internal/handler/endpoint.go`、`frontend/src/views/admin/GroupsView.vue`。
- **迁移与测试**：`backend/migrations/145_group_5h_rate_limits.sql`、`backend/internal/repository/user_group_rate_limit_window_repo_test.go`、`backend/internal/service/admin_service_group_rate_limit_window_test.go`、`backend/internal/service/gateway_profit_control_v2_test.go`、`backend/internal/service/openai_profit_control_paths_test.go`、`backend/internal/handler/admin/user_group_rate_limit_handler_test.go`、`frontend/src/views/admin/__tests__/GroupsView.subscriptionRateLimit5h.spec.ts`。
- **来源提交**：`ae870a2978fc316e51721927d3a91f9bf2f1ceb6`、`17dcffb1c887bf432688e0f25f544b629d4b9eab`、`a80e366bbe27d3212c68ae028cf54cbd714dbe69`、`a6e3a1ceede4cdc048e1f471990b82cc406dd001`、`bbe433256021676ab389439d3bb8157cb0662372`、`7b51c2bd4b53d669d5c37aedaaa9f0ce41edf7df`。
- **当前修复定位**：提交后运行 `git log -S'composite 请求即父分组' -- backend/internal/service/openai_profit_control.go`。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309` 在 `backend/internal/handler/gateway_handler.go` 中保留 `EffectiveGroupRateLimitGroup`、`EffectiveQuotaPlatform` 和 group-rate-limit 记账字段；`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 将这些实际分组不变量与上游 profit gate、`PricingAt` 和倍率计费合并；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 gateway、usage 和计费冲突中继续使用实际选中分组，同时吸收上游搜索、音视频与响应模型计费扩展。
- **合并审查**：重点检查 gateway 预检、账户切换、sticky binding、usage worker 和图像/Grok 分支；同名 `group_id` 不代表已使用实际调度分组。
- **删除条件**：不主动删除。只有产品不再提供分组订阅窗口和 fallback 分组归因时才可移除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run 'Test.*GroupRateLimit')
(cd backend && go test -tags=unit ./internal/service -run 'Test(GatewayProfitControlCompositeSelectionKeepsParentGroupWithoutSyntheticGate|GatewayProfitControlFallbackUsesResolvedGroupRate|ProfitControl_CompositeSelectionKeepsParentGroupWithoutSyntheticGate)')
(cd backend && go test ./internal/repository -run '^TestUserGroupRateLimitWindowRepository')
(cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/GroupsView.subscriptionRateLimit5h.spec.ts)
```

## 3. 账号数据交换与批量账号操作

- **生命周期**：`长期保留`
- **原始意图**：支持 native/CPA 账号数据导入导出、文件级错误定位、批量连接测试、按账号选择模型、最近测试跳过和按失败类型清理账号。
- **行为不变量**：导入不得留下半成功账号或破坏既有代理；CPA schema 自动识别且敏感字段不泄漏；批量测试限制并发并保留 Project 请求头；只有已加载的完整账号集合可进入测试或删除，失败删除结果必须逐项对账。
- **当前代码**：`backend/internal/handler/admin/account_data.go`、`backend/internal/handler/admin/account_data_cpa.go`、`frontend/src/components/admin/account/ImportDataModal.vue`、`frontend/src/components/admin/account/AccountBatchTestModal.vue`、`frontend/src/components/admin/account/AccountBulkActionsBar.vue`、`frontend/src/utils/accountTestRunner.ts`、`frontend/src/views/admin/AccountsView.vue`。
- **测试**：`backend/internal/handler/admin/account_data_handler_test.go`、`frontend/src/__tests__/integration/data-import.spec.ts`、`frontend/src/components/admin/account/__tests__/AccountBatchTestModal.spec.ts`、`frontend/src/components/admin/account/__tests__/AccountBulkActionsBar.spec.ts`、`frontend/src/views/admin/__tests__/AccountsView.batchTest.spec.ts`。
- **来源提交**：`3a295166d17636c9f3b44e53c47a7804ab83819e`、`454639e07532e35a0823048ad18948b508554615`、`3543db03bbcf1750852642115f48b61f7b61bac9`、`affd89e9f8f5c49b0f9909824c779187782ae5ab`、`67a183600a13e8a0170bf6780e2fb60ee3e9501b`、`a6ab6d15cab3d5fb2dcc2d2c65da1c6f6400625b`。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309` 保留 `allSelectedAccountsLoaded` 测试门禁、批量删除结果对账和既有批量测试入口；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 让共享账号测试 runner 同时保留 `X-Project-ID` 并接入上游 Grok 图片、音频和视频测试字段。
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
- **人工合并解决**：无相关人工解决锚点；VERSION-only 提交只是该策略的派生元数据。
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
- **行为不变量**：数据库和 API DTO 均不得重新出现完整 prompt；普通 admin/project admin 不得访问审计路由；缺字段、未知类别、非法 safety/refusal 或多结果不完整时必须 fail closed；清理操作不得删除预览之后的新事件。
- **当前代码**：`backend/internal/securityaudit/prompt_repository.go`、`backend/internal/securityaudit/prompt_event_repository.go`、`backend/internal/securityaudit/prompt_worker.go`、`backend/internal/securityaudit/prompt_qwen3guard.go`、`backend/internal/server/routes/admin.go`、`frontend/src/features/prompt-audit/`。
- **迁移与测试**：`backend/migrations/183_drop_prompt_audit_full_prompt.sql`、`backend/migrations/prompt_audit_privacy_migration_test.go`、`backend/internal/securityaudit/prompt_repository_integration_test.go`、`backend/internal/securityaudit/prompt_qwen3guard_test.go`、`backend/internal/server/routes/prompt_audit_route_coverage_test.go`。
- **来源提交**：`0a6eb610c1f28b738a0b56837ecb44432b6fe739`、`2908160738b82925d3b10854c62092beead2da37`、`86484c52138a4ddabb5ffd0dfcc3791a068202be`。
- **上游部分吸收**：`1b04e03cc4c7c23c216ae0f4830b593700b06eda` 已增加 Responses `output_text` 解析及回归，`c418fd522f429e80c5606d90393d7da601ca30d5` 已增加 WebSocket 最新 turn 审计去重；它们都不覆盖原始 prompt 不落库、路由授权、Qwen3Guard 完整性 fail closed 和并发清理契约，因此本能力继续保留。
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
- **当前代码**：`backend/internal/service/openai_codex_transform.go`、`backend/internal/service/openai_agent_identity.go`、`backend/internal/service/openai_privacy_service.go`、`backend/internal/util/httputil/httputil.go`、`backend/internal/handler/openai_alpha_search.go`、`backend/internal/service/openai_alpha_search.go`、`backend/internal/service/openai_account_scheduler.go`、`backend/internal/service/openai_proxy_stream_circuit.go`、`backend/internal/service/openai_account_runtime_block_fastpath.go`、`backend/internal/service/openai_quota_service.go`、`backend/internal/handler/admin/openai_oauth_handler.go`、`backend/internal/service/gateway_service.go`、`backend/internal/service/openai_gateway_forward.go`、`backend/internal/service/openai_ws_v2/passthrough_relay.go`、`backend/internal/service/openai_ws_v2_passthrough_adapter.go`、`backend/internal/handler/openai_gateway_handler.go`、`frontend/src/components/account/OpenAIQuotaResetCell.vue`、`frontend/src/views/admin/AccountsView.vue`、`frontend/src/utils/openaiEndpointCapabilities.ts`。
- **测试**：`backend/internal/service/openai_codex_transform_test.go`、`backend/internal/service/openai_agent_identity_compat_test.go`、`backend/internal/service/openai_privacy_retry_test.go`、`backend/internal/util/httputil/httputil_test.go`、`backend/internal/handler/openai_alpha_search_test.go`、`backend/internal/service/openai_alpha_search_test.go`、`backend/internal/service/openai_account_scheduler_test.go`、`backend/internal/service/openai_quota_spark_window_test.go`、`backend/internal/service/openai_proxy_stream_circuit_test.go`、`backend/internal/service/openai_account_runtime_block_fastpath_test.go`、`backend/internal/service/openai_ws_v2/passthrough_relay_test.go`、`backend/internal/service/openai_ws_v2_passthrough_lifecycle_test.go`、`backend/internal/handler/openai_gateway_handler_test.go`、`frontend/src/components/account/__tests__/OpenAIQuotaResetCell.spark_shadow.spec.ts`、`frontend/src/views/admin/__tests__/AccountsView.batchTest.spec.ts`。
- **来源提交**：`2dcbd49c92b5affe47c6c7c423650271a50f8209`、`6ed8c0cfb516748d6bffa8a06b5a0586f6e4f3fc`、`16c1da45175d910ae03ca030933eba2286e37b20`、`fc56b7d78728b83fd4cd47dedddbbc055b34040b`、`fea2f5b59dd508df6838074962795d7fc3083a9e`、`83b22ecd2145efb46dae5a5e721f26e4c38a3031`、`e7e0d5a2cfd28940b7eb5f631eb3d7abaacaaf63`、`bfe241b37f5da9d7435506653acf731519718fa4`、`86b122c09c0595fa0cbf6d2cc813fe9a2cda0edf`。
- **上游部分吸收**：`30d2589ef0f0dc839b934b0b21a270d18b7af52b` 已在 ingress lease loss 时保留 terminal event；当前基线还原生包含 compact keepalive `response.failed`（`2f109e74c`）、Responses null tool schema 修复（`f3c94d209`）、pre-output capacity-shed failover（`c33c3208e`、`14a27f196`）、OAuth beta header 移除与 routing hints（`915cc7e7b`、`815035fcc`、`de349187d`）、调度阈值百分比保持（`99b31067f`）、个人订阅过期修正（`358e4a89a`）和 response-model 审计（`db0bff82c`、`6e34fb09c`）。这些上游行为不登记为 fork 差异；它们仍不覆盖 encrypted reasoning、Agent Identity、外部取消 join、Project/代理范围、health-neutral failover 与 quota 部分成功清理契约。
- **人工合并解决**：`0b7eed0738a608971d9711e99ba824d89536f947` 保留 request-scoped proxy quarantine context；`caae38b9abf429d1326ec174b54210a21b023309` 保留 `ShouldUseOpenAIResponsesPassthrough` 和 compact-aware namespace 处理；`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 将 proxy quarantine、passthrough cancellation/close code 与上游 profit admission、load-shed 和 WS turn pricing 合并；`9527e0fc1d85897baf72fbb9ff32027ff3d63aaa` 同时保留 public/fixed model、`ClientLifecycleContext`、`NeutralForAccountHealth`、`RequestScopedTransient` 和项目范围 scheduler cache，并合入上游取消检查与身份/配额恢复；`0bd492e7e7887cec0832981b27c4b164029a6c2c` 让上游 OAuth `count_tokens` HTML 403 fallback 复用 fork 已有的 privacy HTML 响应分类，消除同 package helper 重名且保留两边语义；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 OpenAI gateway、passthrough、WS 和调度冲突中保留项目归属、协议恢复、取消与 health-neutral failover，并合入上游 response-model 审计、容量降载和 routing hints。
- **合并审查**：逐项比较协议测试和状态清理，不得因为上游出现同名 helper 就删除本地行为；特别检查 streaming 已写出后的 failover、credential redaction 和 retry 次数。
- **删除条件**：上游逐项提供等价实现和测试后，可逐项缩减本条；全部不变量均等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test(AdminService_EnsureOpenAIPrivacy|TokenRefreshService_ensureOpenAIPrivacy|ForwardAlphaSearch|OpenAIProxyStreamCircuit|AccountTestServiceOpenAICompactAgentIdentity|OpenAIRuntimeBlock_|ApplyCodexOAuthTransform_)')
(cd backend && go test -tags=unit ./internal/handler -run '^(TestOpenAIGateway|TestAlphaSearch)')
(cd backend && go test ./internal/util/httputil -run '^TestIsCloudflareChallengeResponse$')
(cd backend && go test ./internal/service/openai_ws_v2 -run '^TestRelay_ContextCancellationJoinsBlockedDownstreamWrite$')
(cd backend && go test ./internal/service -run '^TestPassthroughLifecycle_LeaseLossSendsRetryClose$')
(cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run 'Test(ResetCreditLocalClearFailureReturnsConsumedResult|OpenAIGatewayService_ReportOpenAIAccountScheduleFailoverSkipsHealthNeutralFailures|.*ResetQuota)')
(cd frontend && ./node_modules/.bin/vitest run src/components/account/__tests__/OpenAIQuotaResetCell.spark_shadow.spec.ts src/views/admin/__tests__/AccountsView.batchTest.spec.ts)
```

## 8. Grok/xAI OAuth、协议与故障转移正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：修复 Grok OAuth 刷新与轮换凭据、项目隔离、Responses/Chat fallback、客户端工具往返和流式策略错误分类。
- **行为不变量**：refresh token rotation 不得丢失；OAuth 对账不得跨 Project；协议不兼容时只回退可等价的 native Chat；client tool call/result 的 ID 与 payload 必须往返；流式内容策略错误不得误触发跨账号拼接。
- **当前代码**：`backend/internal/service/grok_oauth_reconciliation.go`、`backend/internal/service/oauth_refresh_api.go`、`backend/internal/service/openai_gateway_grok_chat_bridge.go`、`backend/internal/service/openai_gateway_grok_tool_protocol.go`、`backend/internal/service/grok_upstream_errors.go`、`backend/internal/pkg/apicompat/responses_client_tools.go`、`backend/internal/handler/admin/grok_oauth_handler.go`。
- **测试**：`backend/internal/service/grok_oauth_reconciliation_test.go`、`backend/internal/service/openai_gateway_grok_chat_bridge_test.go`、`backend/internal/service/openai_gateway_grok_tool_protocol_test.go`、`backend/internal/service/openai_gateway_response_flush_test.go`、`backend/internal/handler/admin/account_handler_grok_refresh_test.go`。
- **来源提交**：`971a0e5ca717f064afe72a750725f84d43d327fc`、`bce99892926c1e6d83201f74bb82ed74c6353afd`、`88c3138f1a717891eac3276c96eefd44237b45c3`、`e5944b9c7a0230edb0ebd577611c6d849bcb3d68`、`aee1f47c98b60af5fe630be84fc64d4b0e220d3a`。
- **上游部分吸收**：上游 Grok 完整集成由 `fb0475656` 合入，包含 `370bdcf69`、`25d2b03e9`、`e12e0dc1a`、`ec9e73360` 与 `74249b8fe` 等 OAuth、SSO、媒体和协议实现。本条只保留 refresh-token CAS 轮换、Project 隔离、安全 Chat fallback、client-tool 往返和内容策略错误不 failover 五项仍有独立实现与回归的差异。
- **人工合并解决**：`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 Grok OAuth、媒体和 OpenAI bridge 冲突中保留 Project 校验、统一刷新入口、安全 fallback 与实际分组归因，并合入上游 SSO、密码授权和音视频一次性计费。
- **合并审查**：OAuth storage、admin refresh endpoint、scheduler token provider 和 protocol bridge 必须一起审查；只接受上游 UI 或单一 refresh helper 不构成吸收。
- **删除条件**：上游分别提供 OAuth 轮换、项目隔离、协议 fallback 和 client-tool 测试后逐项缩减，全部等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/service -run 'Test(GrokOAuth|OpenAIGateway.*Grok|GrokUpstream)')
(cd backend && go test -tags=unit ./internal/handler/admin -run 'Test.*Grok')
```

## 9. 鉴权、会话绑定与审计状态正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：修复反向代理环境下 session binding 与安全元数据 IP 混用、超级管理员 TOTP 验证方式、多实例审计清理并发、fail-closed 验证码，以及 pending OAuth callback 的前端状态一致性。
- **行为不变量**：会话绑定使用稳定的 binding identity，审计日志仍记录真实安全元数据 IP；TOTP 必须通过用户配置的验证方式；多实例 clear 使用高水位和数据库状态 fencing，清理前已排队或并发写入的审计记录不得越过边界；旧版或自定义 CSP 必须补齐 Aliyun CAPTCHA 的脚本和样式域名，否则不得启用会锁死鉴权入口的 fail-closed 配置；pending OAuth 发送验证码若已经进入 `choose_account_action_required`，所有 callback UI 必须立即消费该服务端状态，不得继续停留在失效的创建账号表单。
- **当前代码**：`backend/internal/pkg/ip/ip.go`、`backend/internal/server/middleware/session_binding.go`、`backend/internal/server/middleware/audit_log.go`、`backend/internal/server/middleware/security_headers.go`、`backend/internal/service/session_binding.go`、`backend/internal/service/totp_service.go`、`backend/internal/repository/audit_log_repo.go`、`backend/internal/service/audit_log_service.go`、`frontend/src/components/auth/PendingOAuthCreateAccountForm.vue`、`frontend/src/views/auth/OidcCallbackView.vue`、`frontend/src/views/auth/LinuxDoCallbackView.vue`、`frontend/src/views/auth/WechatCallbackView.vue`、`frontend/src/views/auth/DingTalkCallbackView.vue`。
- **迁移与测试**：`backend/migrations/182_audit_log_clear_state.sql`、`backend/internal/server/middleware/session_binding_test.go`、`backend/internal/server/middleware/security_headers_test.go`、`backend/internal/service/totp_verification_method_test.go`、`backend/internal/repository/audit_log_repo_test.go`、`backend/internal/repository/audit_log_repo_sequence_integration_test.go`、`backend/internal/service/audit_log_service_test.go`、`backend/migrations/audit_log_clear_state_migration_test.go`、`frontend/src/components/auth/__tests__/PendingOAuthCreateAccountForm.spec.ts`、`frontend/src/views/auth/__tests__/OidcCallbackView.spec.ts`、`frontend/src/views/auth/__tests__/LinuxDoCallbackView.spec.ts`、`frontend/src/views/auth/__tests__/WechatCallbackView.spec.ts`。
- **来源提交**：`4d47d8916691de90d50c454a9935ef5f8a764994`、`8f10a05736dff37c6d275ef33f2fb6b3436ae3ef`、`fb693041dc6fbb4aff7dd4bbf0baa410e2a2ffa3`、`a106870c834e6cf021ffb5adea24c8f2d0ccb1dd`。
- **上游吸收**：`02e50cc22d038dabf3c6af92dbb92d1e0321f8d5` 已完整提供 pending-exchange 服务端账号接管防护及回归，该后端子项不再作为 fork 差异；fork 仍保留创建账号表单立即发出 `complete` 并由各 callback 消费 choice state 的前端契约。
- **人工合并解决**：`8e34f01c53a650a00b80b7bf87476cb74f3118be` 在 CSP 冲突中合并上游腾讯验证码国内/国际区域来源，并保留 fork 的 Aliyun CAPTCHA 脚本与样式来源。
- **合并审查**：区分 trusted proxy 解析、binding key 和 audit IP；审计清理必须同时核对队列 drain、数据库锁顺序与持久化 clear watermark，不能用一个 `ClientIP` 字段重新承担全部语义。
- **删除条件**：上游行为和并发/反代/TOTP 回归测试逐项等价后删除。
- **聚焦验证**：

```bash
(cd backend && go test -tags=unit ./internal/server/middleware -run 'Test.*SessionBinding')
(cd backend && go test -tags=unit ./internal/server/middleware -run 'Test(SecurityHeaders|EnhanceCSPPolicy)')
(cd backend && go test -tags=unit ./internal/service -run 'Test.*(SessionBinding|TOTP|AuditLogService)')
(cd backend && go test ./internal/repository -run '^TestAuditLogRepository')
(cd backend && go test ./migrations -run '^TestMigration182AddsPersistentAuditClearState$')
(cd frontend && ./node_modules/.bin/vitest run src/components/auth/__tests__/PendingOAuthCreateAccountForm.spec.ts src/views/auth/__tests__/OidcCallbackView.spec.ts src/views/auth/__tests__/LinuxDoCallbackView.spec.ts src/views/auth/__tests__/WechatCallbackView.spec.ts)
```

## 10. 支付定价、结果 DTO 与退款状态机正确性

- **生命周期**：`等待上游吸收`
- **原始意图**：分离余额充值倍率与订阅 CNY 定价，以 canonical USD/CNY rate 兼容旧配置；让公开支付结果不泄漏内部 DTO；退款回查以持久化 provider binding/snapshot 为准。
- **行为不变量**：订阅金额只读取 `subscription_usd_to_cny_rate`，legacy multiplier 仅为派生兼容字段；显式 zero/disable 不得复活旧值；公开结果类型不得包含管理端字段；refund finalize 不得从当前订单猜 provider/refund ID，legacy Alipay audit 缺少精确渠道请求 ID 时必须人工核销，旧 pending audit 必须向后兼容，audit 查询失败或内容损坏必须 fail closed；provider 构建、merchant snapshot 校验和本地密钥预检必须先于退款 claim 与权益扣减，退款快照含商户身份或币种而 provider 不提供元数据时必须 fail closed，Wxpay 必须同时报告基础 `appId` 与 JSAPI `mpAppId` 并允许快照匹配实际下单模式使用的任一 AppID，provider 确定性业务失败必须返回 failed 而不能伪装成 pending，Alipay `20000` 或未知错误码等不确定响应必须保留 `REFUNDING` 并复用同一渠道请求 ID；扣减、`REFUNDING` claim 和带 UUID/开始时间的 `dispatching` attempt 快照必须同事务提交，force 余额扣减必须在事务内以 `refundAmount` 为上限按最新可用余额 clamp，不能受 prepare 旧值限制，非 force 余额扣减不足必须整体回滚并要求 force，即时 provider success 不得二次扣减，provider pending 后回查成功也仍须满足同一全额扣减约束；所有 provider outcome、补偿和终态转换必须在事务内校验当前 attempt ID 并重读最新 audit，旧 attempt 的延迟结果或锁前快照不得认领新重试、重复补偿或跳过重新扣减；本地 recovery lease 的 attempt ID 与渠道请求 ID 必须分离：同一次未知结果恢复只轮换 attempt ID 并保持渠道请求 ID，`REFUND_FAILED` 新重试必须生成新的渠道请求 ID；`REFUNDING` 恢复必须等待 5 分钟租约后优先查询 provider，查询与重放分别使用独立的 1 分钟和 3 分钟上下文，只有 Stripe、Alipay、Wxpay、Airwallex 可复用同一渠道请求 ID 重放，query capability 不得绕过该白名单，其余 provider 必须人工核销；provider 明确结果必须先把当前 attempt 原子推进到 `REFUND_PENDING` 并替换 audit，再执行成功或失败终态，未知结果则保留 `REFUNDING`；`REFUND_SUCCESS` 必须保留 attempt ID、渠道请求 ID、退款 ID 和扣减结果后才能删除 mutable pending audit，回查返回的非空退款 ID 必须在终态事务锁定并重读当前 attempt 后合并；legacy pending audit 缺少退款 ID 时，Wxpay 必须按本次退款折算后的渠道金额派生查询 ID，不得使用入账金额；成功和失败终态都必须先原子 claim `REFUND_PENDING`，旧失败查询不得覆盖已提交成功或触发二次退款，失败回查返回的新退款 ID 和 provider failure 必须在锁后重读 attempt，并在终态事务或补偿失败记录事务中重写 pending audit；即使补偿仍失败而保持 `REFUND_PENDING`，也必须在写入 `REFUND_ROLLBACK_FAILED` 的同一事务保留该新退款 ID，失败补偿成功后还必须写入 `deductionRollbackOK=true` 及 `balanceRolledBack`/`subDaysRolledBack`；`REFUND_PENDING` 只能查询或人工核销，不得重新进入 provider refund；`REFUND_FAILED` 重试必须保持原金额、原因、force 和扣减意图，准备阶段必须存在 pending audit 并记录当时订单状态与上一 attempt ID，claim 必须在同一事务内同时匹配该状态和 attempt generation，缺失审计或代际变化必须 fail closed，未补偿的 `REFUND_ROLLBACK_FAILED` 必须在 provider 调用前阻断，补偿成功时必须在同一事务标记 `resolved`；审计表本次先以 expand migration 新增退款状态 `(order_id, action)`、同订单返利 APPLIED/SKIPPED claim 和同订单 `SUBSCRIPTION_ASSIGNED` 三项部分唯一约束，同时保留历史 `(order_id, action)` 全局唯一索引供旧二进制继续匹配无谓词 `ON CONFLICT`；只有下一 fork release 确认旧实例全部下线后，才可用独立 contract migration 删除全局索引并启用普通审计 action 重复追加；进入 pending 的订单状态与当前退款 ID/扣减明细必须通过匹配退款部分唯一索引的原生 upsert 在同一事务内原子替换；余额补偿只能调整 `balance`，不得增加 `total_recharged`；订阅退款必须区分“缩短”与“全量扣减”并可精确补偿，全量扣减必须保留同一订阅行、置为过期并把扣减前后期限写入 audit，失败补偿须在该行锁内合并期间发生的续期，legacy soft-deleted audit 仍可恢复；退款扣减、续期、补偿和兑换码负向调整必须在同一事务内锁定订阅行后基于最新 `expires_at` 重算；幂等分配在锁后发现订阅已被并发请求续期或暂停时，不得再次延长或重新激活；外层事务内不得提前失效订阅缓存，只有 commit 成功后才能同步清除本机 L1、分布式缓存并发布跨实例失效；分布式删除与跨实例发布必须分别使用独立有界上下文并全部尝试，错误合并上报，且缓存失效错误不得记作订阅扣减或补偿失败。
- **当前代码**：`backend/ent/schema/payment_audit_log.go`、`backend/migrations/194_payment_audit_action_idempotency_scopes.sql`、`backend/internal/repository/user_repo.go`、`backend/internal/repository/user_subscription_repo.go`、`backend/internal/service/payment_amounts.go`、`backend/internal/service/payment_config_service.go`、`backend/internal/service/payment_order.go`、`backend/internal/service/payment_order_provider_snapshot.go`、`backend/internal/service/payment_refund.go`、`backend/internal/service/subscription_service.go`、`backend/internal/service/redeem_service.go`、`backend/internal/payment/provider/stripe.go`、`backend/internal/payment/provider/alipay.go`、`backend/internal/payment/provider/wxpay.go`、`backend/internal/payment/provider/airwallex.go`、`backend/internal/handler/payment_handler.go`、`frontend/src/views/admin/SettingsView.vue`、`frontend/src/views/admin/orders/AdminOrdersView.vue`、`frontend/src/views/user/PaymentView.vue`。
- **测试**：`backend/internal/service/payment_config_service_test.go`、`backend/internal/service/payment_order_result_test.go`、`backend/internal/service/payment_order_provider_snapshot_test.go`、`backend/internal/service/payment_refund_test.go`、`backend/internal/service/payment_refund_integration_test.go`、`backend/internal/service/subscription_renewal_lock_test.go`、`backend/internal/service/subscription_revoke_cache_test.go`、`backend/internal/repository/user_subscription_lock_test.go`、`backend/internal/repository/user_repo_integration_test.go`、`backend/internal/repository/user_subscription_repo_integration_test.go`、`backend/internal/payment/provider/stripe_test.go`、`backend/internal/payment/provider/alipay_test.go`、`backend/internal/payment/provider/wxpay_test.go`、`frontend/src/views/admin/__tests__/SettingsView.spec.ts`、`frontend/src/views/user/__tests__/PaymentView.spec.ts`。
- **来源提交**：`cdc7fa66b303333c00b87d5d0852a6d4af9993b7`、`d6f78bf2dc6c11b0c5fe72c4a8f2c92eaf1b7425`、`b5af02ae64ae12254add841f555fbf9538d9eeef`、`427c983f92b1dd7e3815e50bdbdc32caf641cf57`、`51775c230a7575ae0ca70dab751cece53163d35d`。
- **上游相邻行为**：`99b357083e1b5a860f9987523434092e5ef2fcfa` 已原生提供 daily-midnight reset，并将 daily 与 weekly/monthly 使用不同窗口锚点；该行为不登记为 fork 能力，但合并时必须与 fork 的 Project scope、行锁和 commit 后缓存失效同时保留。
- **当前修复定位**：提交后运行 `git log -S'recoverRefundingAttempt' -- backend/internal/service/payment_refund.go`、`git log -S'phase\": \"dispatching' -- backend/internal/service/payment_refund.go` 与 `git log -S'MerchantIdentityMetadata' -- backend/internal/payment/provider/wxpay.go`。
- **人工合并解决**：`caae38b9abf429d1326ec174b54210a21b023309` 在 `payment_config_service.go` 及测试中保留 canonical rate、legacy 派生和输入校验；`d585df8d934807b5eaa3d65aac8cbb2954fa1519` 将上游事务化 pending-refund finalize 与 fork 的 provider snapshot、退款 ID 和旧 pending audit 兼容语义合并；`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在订阅仓储冲突中同时保留 Project 过滤、行锁与缓存失效契约，并合入 daily 与 weekly/monthly 分离的时间锚点。
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
- **原始意图**：避免大表分页重复精确 COUNT，统一筛选口径，暴露 usage-record runtime，恢复已删除 API Key 归因，并确保项目 dashboard 只使用可按项目归因的健康因子。
- **行为不变量**：翻页复用同一筛选条件的精确总数，筛选变化必须失效缓存；列表、统计和错误页使用同一 filter scope；deleted key 只用 digest 归因；项目健康分不得混入全局进程指标；IP geo 仅显式启用。
- **当前代码**：`backend/internal/service/ops_log_runtime.go`、`backend/internal/service/ops_dashboard.go`、`backend/internal/service/ops_health_score.go`、`backend/internal/repository/ops_repo.go`、`backend/internal/repository/usage_log_repo.go`、`frontend/src/views/admin/UsageView.vue`、`frontend/src/views/admin/ops/OpsDashboard.vue`、`frontend/src/components/admin/usage/UsageTable.vue`、`frontend/src/components/common/IpGeoCell.vue`、`frontend/src/utils/ipGeoLookup.ts`。
- **迁移与测试**：`backend/migrations/185_deleted_api_key_audit_digest.sql`、`backend/internal/service/ops_dashboard_test.go`、`backend/internal/service/ops_log_runtime_test.go`、`backend/internal/repository/ops_deleted_key_audit_test.go`、`frontend/src/views/admin/__tests__/UsageView.spec.ts`、`frontend/src/views/admin/ops/__tests__/OpsDashboard.operator.spec.ts`、`frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`、`frontend/src/components/common/__tests__/IpGeoCell.spec.ts`、`frontend/src/utils/__tests__/ipGeoLookup.spec.ts`。
- **来源提交**：`2f78dfc6a00a7cc6de285da6ceabe9868c58a4d7`、`7a17a98b56b7747748f40587cef377bc82143eb2`、`4ec3ddb89297fc8960a8acd9546f72e14bc218f4`、`4cb1350172558b987651097212b22234568dcf79`、`763a01225ffaaf5acd890c765d62a18129f4cee1`、`60cfa88334299f1a471a6eb146668bd46a521e30`、`df0f38f62de8430eb3780bb64716fc645fe2ebc0`、`440386f5ff1ba22c2911e7392177025ef4a42d58`。
- **上游相邻行为**：Channel Monitor V2、upstream response-model 审计（`db0bff82c`、`6e34fb09c`）和系统日志落库退避（`e687ca3e9`）均来自当前上游基线，不登记为 fork 差异；本条只保留精确 COUNT 缓存、deleted-key digest 归因、Project-safe health、共享筛选口径和 opt-in IP geo。
- **人工合并解决**：`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 dashboard、usage query/cache 和 60 列写入冲突中保留 Project scope、共享筛选与精确 COUNT 缓存，同时合入 request type、upstream response-model 和 mismatch 审计。
- **合并审查**：同时核对 query、count、dashboard、error tabs 和权限过滤；前端隐藏全局指标不能替代后端 scope 修复。
- **删除条件**：上游提供等价筛选、COUNT 缓存、deleted-key attribution 和 project-safe dashboard 测试后逐项缩减。
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
- **上游相邻行为**：当前上游已扩展 Grok temporary-unschedulable 状态展示；它不覆盖短错误码/完整 tooltip、Pagination、DataTable row-click 和 iOS 输入缩放契约，因此本条继续保留。
- **人工合并解决**：无相关人工解决锚点。
- **合并审查**：以交互和布局契约为准，不以组件是否仍存在同名 class/prop 为准；桌面表格、移动卡片和虚拟列表都要覆盖。
- **删除条件**：上游分别提供等价交互及回归测试后逐项缩减。
- **聚焦验证**：

```bash
(cd frontend && ./node_modules/.bin/vitest run src/components/account/__tests__/AccountStatusIndicator.spec.ts src/components/common/__tests__/Pagination.spec.ts src/components/common/__tests__/DataTable.spec.ts src/__tests__/iosInputZoom.spec.ts)
```

## 13. 相对部署、示例配置与迁移恢复

- **生命周期**：`等待上游吸收`
- **原始意图**：保证 `/sub2api/api/v1` 等相对 API base 可生成绝对 gateway URL，示例 YAML 可被真实配置加载器解析，让 fork migration `177_account_duplicate_operation_index_notx.sql` 中断后可清理 invalid index 再重试，并避免首次初始化留下无默认 Project owner 的半成品管理员。
- **行为不变量**：浏览器和无 `window` 环境都不得把相对 base 错拼进 Axios；示例 YAML 必须持续通过配置解析；migration `177` 重试不得被上一次 invalid index 永久阻塞；首次 super-admin 与默认 Project owner membership 必须在同一事务提交，membership 未命中唯一默认项目或写入失败时必须整体回滚并允许重试。
- **当前代码**：`frontend/src/api/url.ts`、`deploy/config.example.yaml`、`backend/internal/repository/migrations_runner.go`、`backend/internal/setup/setup.go`。
- **测试**：`frontend/src/api/__tests__/url.spec.ts`、`backend/internal/config/config_test.go`、`backend/internal/repository/migrations_runner_notx_test.go`、`backend/internal/setup/setup_test.go`。
- **来源提交**：`38d6294088369255e79463d836515f7df527f271`、`b255b687ba7872ad7bc590a88a85919cc0253ec9`、`0306521a5557a2520a54c32a014ee7831929397f`。
- **上游相邻行为**：`db0bff82c7f954607f4b66421a79200e150ac836` 为上游 migration `195_add_usage_log_upstream_model_mismatch_index_notx.sql` 增加了同类 invalid-index recovery；当前 runner 同时支持 `177` 与 `195`，但同类机制不等于上游已吸收 fork 的 `177` 迁移兼容契约。
- **人工合并解决**：`a8a3c18641fb1c00030c2baa22fc3918c9e44e68` 在 non-transactional migration runner 冲突中同时保留 fork migration `177` 与上游 migration `195` 的 invalid-index 恢复。
- **合并审查**：相对 URL、示例配置和 migration runner 分别比较；上游只修复其中一项时只删除对应子项。
- **删除条件**：相对 URL、示例配置、migration `177` 恢复和初始 Project owner 原子性分别有等价上游实现与测试后逐项缩减，全部吸收后删除。
- **聚焦验证**：

```bash
(cd frontend && ./node_modules/.bin/vitest run src/api/__tests__/url.spec.ts)
(cd backend && go test ./internal/config -run '^TestExampleConfigIsValidYAML$')
(cd backend && go test ./internal/repository -run '^TestApplyMigrationsFS_NonTransactionalMigration')
(cd backend && go test ./internal/setup -run '^TestCreateInitialAdminRollsBackAndRetriesWhenDefaultProjectMembershipIsMissing$')
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
- **已替代方案**：`6b37423465f1780dda62232d88e53676c4af15d5` 与 `bb9e60b13bab1a7f7c31d8987dfa00d5ed6da8ef` 的旧 operator/group-scope 方案已由 Project 模型替代，不单列。
- **派生输出**：单纯 VERSION 同步、Ent/Wire 生成输出和 locale 补齐不单列；它们归属于对应 source capability。
- **内容与机械变更**：赞助商文案、链接修正、格式、lint annotation 和仅测试适配不单列。
- **已合并上游修复**：`21aacde0b3d340e21253b73a04f6e724b40a77de` 已通过上游父提交 `b74024c7868ee88a0bf921306cbc22a2f922872a` 进入当前基线；其让下行 write context 脱离 relay cancellation 的基础修复不是 fork 差异。当前 fork 额外保证外部取消会关闭连接并 join 阻塞写，该行为归入能力 7，不单列新能力。
- **本次上游吸收**：上游父提交 `27e8f69a9e04d5919c7f4b6a4175c34af24e7eb2` 已提供 Stripe 金额级幂等键、pending refund 的事务化 claim/finalize、可用余额原子扣减，以及 Messages 临时账号错误切换；这些上游子能力不作为 fork 差异。能力 7 与 10 只保留仍超出上游的协议、Project、provider snapshot、退款审计兼容等不变量。
- **本次上游部分吸收**：上游父提交 `00b8596176809906993169c283671811ad04f58d` 包含 `1b04e03cc4c7c23c216ae0f4830b593700b06eda` 的 Responses `output_text` 解析和 `30d2589ef0f0dc839b934b0b21a270d18b7af52b` 的 lease-loss terminal event 保留；能力 5 与 7 只移除这些重叠子项，其余隐私、授权、fail-closed、取消、代理、故障转移和配额清理契约继续保留。
- **本次上游新增**：Composite 分组模型广场、Codex WebSocket prewarm continuation、OAuth `count_tokens` HTML 403 fallback、Grok 视频 `task_id`、Gemini 3.6 Flash 模型、Ops 自定义错误时间范围，以及 upstream transport / SOCKS5 的 TCP 建连超时均来自当前上游基线，不登记为 fork 能力；`count_tokens` 人工解决仅复用 fork 已有的 privacy HTML 响应分类。
- **当前基线上游原生能力**：Composite 图片权限（`9b54b46b0`，由 `5deeb7ef1` 合入）、daily-midnight subscription reset（`99b357083`）、Channel Monitor V2、upstream response-model 审计、系统日志退避、Grok 完整集成和 backup multipart 行为均来自 `10a4c6e3ad319587e817109c071259269855ec30` 基线，不登记为 fork 能力，也不得在后续冲突中因同名本地 helper 而删除。
