# 6URL SEO / GEO 导航平台改造方案

> 状态：已评审，正在实施。当前实施分支：`codex/directory-platform-foundation`。

**日期**：2026-08-09

**项目**：6URL

**目标**：把当前 JSON 驱动的静态导航页升级为具备内容管理、申请审核、合作链接检查、付费收录和运营 API 的 SEO / GEO 优先导航平台。

## 1. 背景与现状

改造开始前的项目采用 Next.js 13 Pages Router：

- 网站配置来自 `pages/api/site.json`。
- 分类和网站数据来自 `pages/api/resources.json`。
- `pages/api/index.ts` 在构建时直接导入 JSON。
- `pages/components/content.tsx` 直接遍历 JSON 生成首页卡片。
- 项目没有数据库、申请记录、审核状态、订单、定时任务和运营后台。
- `next/image` 仅允许 `next.config.js` 中列出的少量远程图片域名。

当前模式适合少量人工维护。新的商业模式需要支持两种收录来源：

1. 合作链接收录：申请方在其网站放置 6URL 链接，平台定期验证合作状态。
2. Paid submission：申请方完成编辑预审和付款后获得标准收录资格。

项目对 SEO 和 GEO 高度依赖，因此内容稳定性、索引质量、实体可信度和商业链接合规优先于短期发布速度。

## 2. 设计原则

### 2.1 SEO / GEO 是发布门槛

公开页面必须提供服务端可见的完整主体内容。首页、分类页和网站详情页采用静态生成或增量静态再生成，搜索引擎和 AI 搜索系统无需执行客户端请求即可读取核心信息。

### 2.2 内容实体与收录资格分离

`sites` 表示可被搜索和引用的网站实体，`placements` 表示该实体当前拥有的收录资格。付款到期和合作链接异常只改变 placement；已经通过编辑审核的网站详情页保持稳定。`ads` 独立表示 Sponsor（Ad，广告），不参与收录资格和目录 submission 状态机。

这项拆分解决以下问题：

- 暂时性商业状态变化不会制造大量 404。
- 站点详情 URL、canonical 和实体 ID 长期稳定。
- 固定期限的 Paid submission 到期后可以结束收录资格，同时保留有价值的编辑内容。
- 合作链接恢复后只需恢复 placement，无需重新创建详情页。

### 2.3 编辑审核决定公开质量

申请人提交的标题、简介和卖点属于审核素材。公开页面使用编辑确认后的独立内容，避免重复营销文案、关键词堆砌、虚假陈述和批量薄内容。

### 2.4 商业关系透明

平台明确区分编辑收录、合作链接收录、付费收录和联盟链接。链接属性、页面徽章和披露文案由关系类型决定。

### 2.5 数据和页面状态可追溯

申请、审核、付款、发布、反链检查、过期和人工操作均保留时间、操作者和原因，支持审计、申诉与故障定位。

## 3. 目标与范围

### 3.1 首版目标

- 数据库管理分类、网站内容和收录资格。
- 提供首页、分类页、网站详情页三层可索引结构。
- 提供公开提交表单和邮件状态链接。
- 支持 Paid submission 与 Link exchange submission 两条公开申请流程。
- 支持管理员免费直录，跳过公开 submission、订单和支付流程。
- 支持用户主动申请 Sponsor（Ad，广告）并在线支付，也支持管理员免费配置广告。
- 广告申请、广告订单和广告展示与 Paid submission 使用独立业务状态；底层复用 Stripe、通知和审计基础设施。
- 提供完整管理 API，供本地 AI 完成审核、编辑、发布、上下架和排序。
- 提供 OpenAPI 3.1 文档、本地安全 token 和适合 AI 调用的稳定错误结构。
- 提供简单管理后台，用于数据查看、筛选、预览和常用快捷操作。
- Stripe Checkout 支持一次性付款；管理 API 支持人工收款确认。
- 每日检查合作链接，使用宽限期降低误暂停。
- 提供 sitemap、canonical、结构化数据、重定向和内容发布门槛。
- 多语言路由、内容审核、hreflang、locale sitemap 和语言切换形成首版闭环。
- 建立可观测的 SEO / GEO 指标。

### 3.2 后续范围

- 对比页 `/compare/{a}-vs-{b}`。
- 指南、榜单和专题内容 `/guides/{slug}`。
- 申请方账号中心。
- 自动续费订阅。
- 站内搜索和个性化推荐。

首版实现完整多语言内容工作流，明确支持 `zh-CN`、`en`、`ja`、`ko`。数据模型通过 locale 表保留后续扩展能力。每种语言独立审核、独立发布和独立进入索引。

## 4. 关键产品决策

| 决策 | 首版建议 | 状态 |
|---|---|---|
| 正式站点域名 | `https://6url.net` | 已确认 |
| 公开收录申请类型 | Paid submission、Link exchange submission | 已确认 |
| 付费套餐数量 | 单一套餐 | 已确认 |
| Sponsor 定义 | Ad（广告），与 Paid submission 分离 | 已确认 |
| 广告获得方式 | 用户主动申请并付款；管理员也可免费配置 | 已确认 |
| 管理员广告 | 跳过广告申请、订单与支付流程 | 已确认 |
| 当前广告 | 线上现有 4 个 Sponsor 广告配置迁移进入数据库 | 已确认 |
| 单一 Paid submission 有效期 | 一次性永久收录或固定期限 | 待确认 |
| 付费时机 | 编辑预审通过后创建支付 | 建议确认 |
| 合作链接定位 | 合作资格信号，接受自然品牌链接 | 建议确认 |
| 合作链接属性 | 6URL 出站链接使用 `nofollow` | 建议确认 |
| 付费链接属性 | 6URL 出站链接使用 `sponsored` | 建议确认 |
| 编辑主动收录 | 独立审核后允许普通链接 | 建议确认 |
| 反链失败宽限 | 连续 3 次且跨度达到 72 小时 | 待确认 |
| 付费到期处理 | 结束商业 placement，保留合格详情页 | 建议确认 |
| 申请人身份 | 邮箱 + 高熵状态令牌 | 建议确认 |
| 状态令牌传递 | 邮件链接使用 URL fragment，换取短期 HttpOnly application session | 建议确认 |
| 支付渠道 | Stripe + 管理 API 人工收款 | 待确认 |
| Stripe 结算币种 | 收录与广告订单均使用 USD | 已确认 |
| 首版退款能力 | 仅支持全额退款，退款后结束对应 paid placement | 建议确认 |
| 首发语言 | `zh-CN`、`en`、`ja`、`ko` | 已确认 |
| 多语言 URL | 默认中文保留无前缀路径，其他语言使用 `/{locale}` | 建议确认 |
| 多语言发布 | 每个 locale 独立审核；缺失语言不生成页面 | 建议确认 |
| 收录资格语言 | 套餐显式保存包含的 locale；首版建议覆盖四种语言 | 待确认 |
| 管理方式 | 简单管理后台 + 完整管理 API；复杂操作主要由本地 AI 调用 | 已确认 |
| 管理员免费直录 | 后台 / API 直接创建并发布，跳过 submission、订单与支付 | 已确认 |
| 管理鉴权 | 服务端保存 token hash，本地保存原始 token | 已确认 |
| 管理员线上通知 | Slack | 已确认 |
| 收录上线通知 | 正式发布后向申请邮箱发送页面链接 | 已确认 |
| 生产环境配置 | 由项目所有者配置环境变量与密钥 | 已确认 |
| 图片托管 | ImgAnt `6url` 相册，本地 AI 上传后登记 | 已确认 |
| 多重商业关系 | 按 paid / affiliate、exchange、editorial 的固定优先级生成 CTA、rel 与披露 | 建议确认 |
| 用户广告产品 | 单一价格、展示期限、可售位置与库存规则 | 待确认 |

单一 Paid submission 的 USD 价格、有效期、退款条款和审核 SLA 在实施前完成业务确认。

## 5. 信息架构

```text
/
├── /categories/{category-slug}
├── /sites/{site-slug}
├── /en
├── /en/categories/{category-slug}
├── /en/sites/{site-slug}
├── /ja
├── /ja/categories/{category-slug}
├── /ja/sites/{site-slug}
├── /ko
├── /ko/categories/{category-slug}
├── /ko/sites/{site-slug}
├── /submit
├── /en/submit
├── /{locale-prefix?}/submissions/status
├── /advertise
├── /en/advertise
├── /{locale-prefix?}/ad-applications/status
├── /editorial-policy
├── /listing-policy
├── /advertising-policy
├── /corrections
├── /sitemap.xml
├── /sitemaps/categories.xml
├── /sitemaps/sites-{n}.xml
├── /feed.xml
├── /llms.txt
├── /admin（简单管理后台）
└── /admin/v1/openapi.json（管理 API 描述，仅鉴权后访问）
```

默认语言 `zh-CN` 继续使用现有无前缀根路径，保留当前首页 URL 和未来积累的链接权益。其他语言统一使用 locale 前缀。网站和分类 slug 在语言间保持一致，页面展示名称、标题、正文和 metadata 按 locale 管理。

正式 canonical origin 为 `https://6url.net`，默认中文页面直接使用该 origin。示例对应关系：

```text
zh-CN  https://6url.net/sites/notion
en     https://6url.net/en/sites/notion
ja     https://6url.net/ja/sites/notion
ko     https://6url.net/ko/sites/notion
```

语言切换器输出普通 `<a>` 链接。用户选择可以写入 cookie，服务端保持 URL 对应内容稳定，页面不会依据浏览器语言在同一 URL 返回不同正文。

### 5.1 首页

首页承担品牌、核心分类、精选网站和最新更新入口。页面控制卡片总量，避免把全部数据库一次性输出为超长页面。

核心模块：

- 一句话说明网站定位。
- 核心分类入口。
- 编辑精选。
- Sponsor 广告区域，展示当前有效广告并提供明确广告标识；首批数据包含现有 4 个管理员配置项。
- 最新通过审核的标准收录。
- 最近更新的网站。
- 收录标准与提交入口。

### 5.2 分类页

每个分类拥有独立介绍、选择标准、网站列表、相关分类和常见问题。分类介绍由编辑维护，支持长文本和来源引用。

分类页分页使用可抓取 URL，例如 `?page=2` 或 `/page/2`。首版建议采用 URL 参数并为每页设置自引用 canonical。

### 5.3 网站详情页

每个公开网站拥有稳定 URL。详情页是 SEO / GEO 的主要实体页面，包含：

- 规范名称、Logo、官方域名和一句话定位。
- 独立摘要与完整介绍。
- 核心功能。
- 适用人群和使用场景。
- 价格摘要与价格信息更新时间。
- 优点、限制和替代产品。
- 官方链接与事实来源。
- 所属分类和相关网站。
- 首次审核、最近编辑和最近核验时间。
- 商业关系披露。
- 真实存在且对用户有帮助的 FAQ。

### 5.4 政策页

`/editorial-policy` 解释选站标准、内容更新和纠错机制。`/listing-policy` 解释编辑收录、合作收录、付费收录、链接属性、期限和退款边界。`/advertising-policy` 解释 Sponsor 广告申请、审核、标识、展示期限、退款与下架规则。政策透明度同时服务用户信任、搜索质量和 AI 引用判断。

### 5.5 多语言内容体系

多语言采用“共享实体事实 + locale 独立编辑内容”：

- `sites` 保存域名、官方 URL、facts version 等跨语言事实。
- `site_content_revisions` 保存每种语言的不可变正文版本；`site_locale_publications` 分别指向当前草稿和已发布版本。
- `categories` 保存层级、稳定 slug 和排序。
- `category_content_revisions` 保存分类的不可变语言版本；`category_locale_publications` 保存草稿与发布指针。
- 来源可以跨语言共享，也可以通过 locale 标记本地化来源。
- 商业 placement 在语言间共享有效期，具体展示文案与披露按 locale 输出。
- placement 通过 `placement_locales` 声明覆盖范围，并与该 locale 已发布内容取交集后展示。

内容工作流：

```text
source draft
  └── translation_requested
        └── translated_draft
              └── locale_review
                    ├── needs_changes
                    └── approved ──► published
```

每种语言独立满足发布门槛。某个 locale 缺少已审核内容时，该语言 URL 保持不存在，并从 hreflang、sitemap、目录 API 和内部链接中排除。系统不会在英文 URL 上回退展示中文正文。

任一 source locale 的事实或正文变更后，把由它派生且版本落后的语言标记为 `source_changed`。管理 API 返回待同步差异，编辑确认翻译和事实一致后创建新 revision；现有 published revision 在新版本通过审核和发布前保持稳定。存量 JSON 迁移以 `zh-CN` 作为首个 source locale，后续内容允许从任一受支持语言发起。

## 6. 渲染与索引策略

| 页面 | 渲染 | 缓存 / 刷新 | 索引策略 |
|---|---|---|---|
| 各语言首页 | SSG + ISR | 5 分钟；发布后按 locale 刷新 | `index,follow` |
| 各语言分类页 | SSG + ISR | 10 分钟；分类变更后按 locale 刷新 | `index,follow` |
| 各语言网站详情页 | SSG + ISR | 1 小时；发布后按 locale 刷新 | 该 locale 通过质量门槛后 `index,follow` |
| 提交页 | SSR / 静态 | 常规 CDN | `noindex,follow` |
| 提交状态页 | 动态 | 私有、禁止共享缓存 | `noindex,nofollow` |
| 支付结果页 | 动态 | 私有、禁止共享缓存 | `noindex,nofollow` |
| 简单管理后台 | CSR / 动态 | 私有、禁止共享缓存 | `noindex,nofollow` |

### 6.1 发布门槛

详情页满足以下条件后进入 sitemap 并允许索引：

- canonical 域名、规范 URL 和 slug 已确认。
- 名称、摘要和正文经过编辑审核。
- 内容与站内其他页面具有足够差异。
- 至少一条可核验来源；涉及价格、公司和关键功能时增加对应来源。
- 分类、关系类型、链接属性和披露信息完整。
- Logo 已上传到 ImgAnt 的 `6url` 相册，图片尺寸和替代文本完整。
- 页面结构化数据通过校验。
- 页面没有阻断性链接错误或安全风险。
- 当前 locale 已完成独立审核，语言、metadata 和可见正文一致。

从 JSON 导入的存量网站可以立即保留首页卡片。新建详情页先保持 `noindex`，完成编辑门槛后逐批发布，防止一次性生成大量薄页面。

### 6.2 状态与 HTTP 行为

| 内容状态 | HTTP / 索引行为 |
|---|---|
| `draft` | 无公开路由或返回 404 |
| `published` | 200，允许索引，进入 sitemap |
| `placement_suspended` | 详情页保持 200；暂停推荐展示和商业 CTA |
| `content_under_review` | 已发布页面保持 200；显示最近已核验内容 |
| `archived_with_value` | 200，展示历史状态和替代项，可继续索引 |
| `removed` | 有直接替代时 301；永久删除且无替代时 410 |
| slug 变更 | 旧 slug 永久 301 到新 slug |

页面公开状态由内容质量决定，placement 状态由收录资格决定。

### 6.3 按需刷新

Go 后端采用 transactional outbox：内容发布状态、locale publication 指针和 `revalidation_jobs` 在同一个 MySQL 事务中写入。事务提交后，Worker 读取 outbox 并使用带签名的内部接口通知 Next.js：

```text
POST /api/internal/revalidate
{
  "paths": ["/", "/sites/example", "/en", "/en/sites/example"],
  "event_id": "...",
  "timestamp": 1786240000
}
```

- 使用 HMAC 签名、时间窗口和 event ID 防重放。
- 刷新失败进入重试队列。
- Worker 使用 job ID 抢占、超时回收和幂等完成标记，数据库提交成功时刷新任务一定可恢复。
- ISR 时间窗口承担最终兜底。
- 内容下架、slug 变更和链接属性变化均触发刷新。

## 7. SEO 技术规范

### 7.1 URL 与 canonical

- slug 使用小写 ASCII 和连字符。
- 每个实体在每个已发布 locale 下只保留一个 canonical URL。
- 公开 canonical origin 固定为 `https://6url.net`；HTTP 和 `www.6url.net` 使用单跳 301 到对应 HTTPS 无 `www` 路径。
- `zh-CN` 使用无前缀路径；`en`、`ja`、`ko` 使用固定语言前缀。
- URL 规范化统一处理协议、主机大小写、尾斜杠、默认端口和追踪参数。
- 旧 slug 写入 `redirects`，永久 301 到当前 URL。
- 筛选参数页默认不进入索引；拥有独立搜索价值的组合页经过编辑后升级为固定落地页。

### 7.2 Metadata

每个可索引页面存储或生成：

- 唯一 `<title>`。
- 唯一 meta description。
- canonical。
- Open Graph 和 Twitter Card。
- `robots` 指令。
- 当前页面语言和完整 hreflang 集合。

标题和描述采用预览与重复检测。字符数量只作为编辑提示，内容准确性和点击意图优先。

### 7.3 Hreflang 与语言切换

- `zh-CN`、`en`、`ja`、`ko` 使用标准 BCP 47 locale。
- `<html lang>` 分别输出 `zh-CN`、`en`、`ja`、`ko`，Open Graph locale 使用对应格式。
- 每个已发布翻译输出 self-reference 和所有已发布对应语言。
- hreflang 关系保持双向；缺少对应页面时不输出该语言。
- `x-default` 指向默认中文页面。
- HTML `<head>` 与 sitemap 中的 alternate 集合来自同一服务，避免漂移。
- 语言切换使用可抓取链接，并保留当前实体或分类。
- 搜索引擎和用户访问明确 locale URL 时直接返回该语言，服务端不会根据 IP 改写正文。
- `/en`、`/ja`、`/ko` 下的路径和页面语言保持一致。

### 7.4 结构化数据

| 页面 | JSON-LD |
|---|---|
| 全站 | `WebSite`、`Organization` |
| 分类页 | `CollectionPage`、`ItemList`、`BreadcrumbList` |
| 网站详情 | `WebSite` 或匹配实体类型、`BreadcrumbList` |
| 软件类详情 | 信息充分时使用 `SoftwareApplication` |
| FAQ | 页面真实展示 FAQ 时使用 `FAQPage` |

结构化数据与页面可见内容保持一致。类型选择依据实体真实属性，避免为覆盖关键词而堆叠 schema。

JSON-LD 的 `inLanguage`、名称、描述和面包屑使用当前 locale；实体 `@id` 使用稳定站内标识，使四种语言引用同一个底层实体。

Sponsor 广告保持服务端可见并明确披露，同时从编辑精选、分类 `ItemList`、相关推荐和网站实体评价的 JSON-LD 中排除。公开 catalog API 将广告放入独立 `ads` 字段，避免广告关系被机器消费者解释为编辑推荐或自然排序信号。

### 7.5 Sitemap 与 robots

- sitemap 仅包含 canonical、200、允许索引的 URL。
- sitemap 分片控制在协议限制内，并按内容类型拆分。
- sitemap 为每个 URL 输出已发布的 `xhtml:link hreflang` alternates。
- sitemap 生成器校验 alternate 双向完整性和 locale URL 语言一致性。
- `lastmod` 来自实质内容更新时间，商业排序变化不修改详情页 `lastmod`。
- `robots.txt` 声明 sitemap，并阻止后台、支付和私有状态路径抓取。
- AI 搜索爬虫策略使用配置管理；搜索发现、模型训练和普通搜索分别评审。

### 7.6 内链

- 各语言首页链接当前 locale 的核心分类与编辑精选。
- 分类页链接当前 locale 已发布的实体页面。
- 详情页链接当前 locale 的所属分类、相关网站、替代产品和政策说明。
- 相关网站由分类和编辑关系共同决定，首版避免纯关键词自动生成。
- 定期扫描孤立页面、循环重定向和失效内部链接。

### 7.7 性能目标

以真实用户第 75 百分位为验收口径：

- LCP ≤ 2.5 秒。
- INP ≤ 200 毫秒。
- CLS ≤ 0.1。
- 主体内容无需客户端 API 请求即可出现。
- 首页和分类页限制第三方脚本、字体和大图数量。

## 8. GEO 设计

GEO 关注实体清晰度、答案完整性、事实来源、稳定 URL 和内容可引用性。

### 8.1 实体数据

每个网站实体保存：

- 稳定内部 ID。
- 规范名称和别名。
- 官方域名、canonical URL 和 `sameAs`。
- 类型与分类。
- 核心功能、适用场景、价格事实和更新时间。
- 来源、编辑人和核验时间。
- 商业关系类型。

### 8.2 可引用内容结构

详情页使用直接回答式内容：

- “X 是什么”摘要。
- “适合谁”列表。
- “主要功能”事实列表。
- “价格与限制”带日期的说明。
- “替代方案”内部实体链接。
- “信息来源”可见引用列表。

重要事实使用结构化字段生成稳定表述，同时保留人工编辑正文。页面避免隐藏内容、折叠后才加载的主体信息和只有图标的功能描述。

### 8.3 机器可读入口

- sitemap。
- RSS / Atom 更新源。
- 公开只读目录 API，提供版本、更新时间和稳定实体 ID。
- `llms.txt` 作为实验性发现入口，内容指向政策、分类和高质量实体页。

`llms.txt` 当前属于辅助实验，搜索可访问性、内容质量和来源可信度承担主要效果。

### 8.4 GEO 衡量

- 记录来自 AI 搜索产品的可识别 referral。
- 维护固定问题集，定期人工采样品牌与分类答案中的引用情况。
- 统计 AI crawler 的访问、状态码、抓取页面类型和失败率。
- 观察品牌词、实体名称和长尾问题的 Search Console 表现。
- 将引用准确性和事实过期率纳入内容运营指标。

## 9. 商业链接与搜索风险

### 9.1 关系类型

`placements.kind` 使用以下枚举：

- `editorial`：编辑独立选择并审核。
- `exchange`：以合作链接作为持续资格。
- `paid`：通过 Paid submission 获得标准收录资格。
- `affiliate`：6URL 通过出站转化获得佣金。
- `owned`：平台自有或关联产品。

### 9.2 出站链接策略

| 关系 | 默认属性 | 披露 |
|---|---|---|
| `editorial` | `noopener`，独立审核后允许普通链接 | 编辑收录 |
| `exchange` | `nofollow noopener` | 合作收录 |
| `paid` | `sponsored noopener` | 付费收录 |
| `affiliate` | `sponsored noopener` | 联盟链接披露 |
| `owned` | `noopener` | 关联关系披露 |

合作链接计划使用自然品牌名，平台不规定关键词锚文本，不承诺排名提升。申请方页面允许使用 `nofollow`；资格检查关注公开可见的真实合作标识。

同一 site 可以同时存在多条 active placement。页面按以下固定规则生成主 CTA、链接属性与披露：

1. 任一有效 `paid` 或 `affiliate` 关系参与当前展示时，主 CTA 使用 `sponsored noopener`，同时披露所有参与展示的商业关系。
2. 没有付费或联盟关系且有效 `exchange` 参与当前展示时，主 CTA 使用 `nofollow noopener` 并披露合作收录。
3. `editorial` 普通链接只在完成独立编辑审核且当前展示不受上述商业关系驱动时使用 `noopener`。
4. 多条同级 placement 按开始时间和 ID 稳定决胜；披露按关系类型固定排序并去重。

服务层输出已经解析的 `effective_outbound_url`、`effective_rel` 和 `disclosures`，前端不自行推断关系优先级。

`rel="sponsored"` 承担搜索引擎的付费或广告链接合规标记。Paid submission 只获得标准收录，不自动成为广告；用户界面使用“付费收录 / Paid submission”披露。Sponsor 广告卡片独立使用 `sponsored noopener` 和“Sponsor / 广告”标识，即使该广告由管理员免费配置。

现有 `resources.json` 中包含重定向和疑似联盟链接。迁移前逐条确认最终域名、商业关系和链接属性。

### 9.3 排序规则

- Paid submission 与 Link exchange submission 都进入标准收录列表。
- 列表采用质量、相关性、更新时间和人工排序。
- 付费金额不直接改变编辑评价文本。
- 分类页完全依据 `site_categories` 的编辑分类，placement 只决定当前是否具备收录资格。
- 排序算法和两种申请的展示规则写入 listing policy。

### 9.4 Sponsor（Ad，广告）

- Sponsor 就是 Ad，是首页独立广告模块；当前 4 个线上配置作为首批迁移数据。
- 用户可通过独立广告入口提交申请；审核通过并确认付款后创建广告。
- 管理员可通过后台或管理 API 免费创建、排序、启停广告和设置可选有效期，跳过广告申请、订单与支付。
- 广告不关联目录收录的 `submissions`、`listing_plans` 或 `placements`；用户广告订单通过 `ad_application_id` 与广告申请关联。
- 广告保存独立素材、目标 URL 和四语言文案，`site_id` 仅作为可选关联，因此广告可以推广目录外页面。
- 广告排序只影响 Sponsor 广告模块，不改变首页标准收录、分类页或详情页的编辑排序。
- 广告卡片固定展示“Sponsor / 广告”标识，出站链接固定使用 `sponsored noopener`。

## 10. 用户流程

### 10.1 公共提交

1. 申请人选择 Paid submission 或 Link exchange submission。
2. 填写网站名称、官方 URL、联系邮箱、分类、简介、Logo 和相关资料。
3. 合作链接类型额外填写放置 6URL 链接的页面 URL。
4. 服务端执行 Turnstile、限流、URL 规范化、重复检查和基础安全检查。
5. 创建 submission 并发送状态链接；高熵 token 放在 URL fragment 中，例如 `/submissions/status#token=...`。
6. 管理员收到新申请通知。

重复 URL 进入现有实体更新流程，避免为同一网站创建多个详情页。

状态页加载后由客户端读取 fragment，并通过单次 `POST /api/v1/submission-session` 请求换取 30 分钟有效的 HttpOnly cookie。fragment 不会随 HTTP 请求、Referer、CDN 日志或服务端访问日志发送；交换成功后使用 `history.replaceState` 清除地址栏 token。交换接口采用严格限流，原始 token 请求体和 hash 输入均从应用日志、APM body capture 与错误上报中排除。

### 10.2 编辑预审

管理员可以：

- 要求补充材料。
- 修改分类和建议 slug。
- 标记重复实体。
- 拒绝违法、欺诈、低质量或超出范围的网站。
- 编辑公开内容并预览 SEO 页面。
- 通过预审并进入对应商业流程。

申请状态：

```text
received
  ├── needs_changes ──► received
  ├── rejected
  └── preapproved
        ├── exchange_verification
        └── awaiting_payment
```

### 10.3 Link exchange submission

1. 预审通过后立即检查 backlink URL。
2. 检查通过后管理员完成内容发布审核。
3. 创建或更新 site，创建 `exchange` placement。
4. placement 进入 `active`，前台显示“合作收录”。
5. Worker 每日按随机抖动时间复查。
6. 连续 3 次失败且跨度达到 72 小时后进入 `suspended`。
7. 暂停首页、分类列表中的合作展示和合作 CTA，并发送通知。
8. 链接恢复后自动进入 `recovered_pending_publish`；系统刷新页面，管理员可配置自动恢复。

首版建议自动恢复 placement，同时保留人工暂停开关。

### 10.4 Paid submission

1. 编辑预审通过。
2. 系统创建 order，保存套餐名称、价格、币种、期限和权益快照。
3. 后端创建 Stripe Checkout Session。
4. Stripe webhook 验签并按 event ID 幂等处理 `checkout.session.completed`、`checkout.session.async_payment_succeeded`、`checkout.session.async_payment_failed`、`checkout.session.expired`、`refund.created`、`refund.updated`、`refund.failed` 和 `charge.refund.updated`；其余事件返回 200 ignored 且不写事件表。
5. 6URL 与其他产品共享 Stripe 账号，每个新 Checkout Session、PaymentIntent 和 Refund 写入 `sixurl_app=6url.net` 与 `sixurl_order_id`。Webhook 通过专属 metadata 加本地 Checkout Session / PaymentIntent 双重归属校验；历史 Checkout 使用已保存的 Session ID 兼容，其他产品事件返回 200 ignored。归属确认后继续校验金额、币种、livemode、Checkout Session 和 PaymentIntent，只有 Stripe 明确确认 paid 后才把 order 标记为 `paid`。
6. 创建或更新标准收录的 paid placement，设置 `starts_at`；固定期限套餐同时设置 `ends_at`，永久收录保持为空。
7. 发布页面并发送收据与状态链接。
8. 固定期限套餐在到期前 30、7、1 天发送提醒。
9. 固定期限套餐到期后结束 paid 收录资格，网站详情页按编辑状态继续保留；永久收录跳过到期流程。
10. 首版退款只支持全额退款；退款确认后把 order 标记为 `refunded` 并在同一业务事务中结束对应 paid placement。

浏览器支付回跳用于展示结果，订单状态由 webhook 和管理 API 人工确认决定。

### 10.5 人工收款

管理员填写渠道、外部流水号、金额、币种、收款时间和备注。系统要求外部流水号唯一，并写入审计日志。人工确认与 Stripe 订单共享 placement 激活逻辑。

### 10.6 管理员免费直录

1. 管理员通过后台或 `/admin/v1` 直接创建 site，来源标记为 `admin_manual`。
2. 编辑分类、四语言内容、来源和 Logo，继续执行与用户申请相同的内容质量预检。
3. 创建 `editorial` placement，`origin=admin`、`commercial=false`，不创建 submission、order 或 payment event。
4. 发布通过审核的 locale，并通过 transactional outbox 刷新页面。
5. 管理员可以选择填写联系人邮箱并发送上线邮件；缺少联系人邮箱不阻断免费直录。

### 10.7 管理员配置 Sponsor 广告

1. 管理员创建广告，填写目标 URL、图片和至少一个 locale 的标题、说明、CTA 与 alt 文本。
2. 可选关联现有 site；广告也可以独立推广目录外页面。
3. 管理员设置 locale 覆盖、排序、启停状态和可选有效期，不创建 submission、order 或 payment event。
4. 发布预检强制广告标识、`sponsored noopener`、安全目标 URL 和已登记 ImgAnt asset。
5. 保存事务同时创建首页 revalidation job；前台 Sponsor 广告模块更新。

### 10.8 用户申请 Sponsor 广告

1. 广告主在独立 `/advertise` 页面填写联系人邮箱、目标 URL、原始图片 URL 或待审核素材、期望 locale 和广告文案；该入口不会创建目录收录申请。
2. 服务端执行 Turnstile、限流、目标 URL 与素材 URL 安全检查和重复申请检查，创建 `ad_application` 并发送状态链接，同时向管理员发送 Slack 通知。
3. 管理员审核广告主、目标页面、素材、文案和展示期限，必要时要求补充资料；通过后按广告产品快照创建 USD 订单。
4. 广告主通过独立广告申请 session 创建 Stripe Checkout；订单只由签名 webhook 或管理 API 人工确认驱动。
5. 付款确认后创建 `origin=advertiser` 的广告草稿；管理员或本地 AI 把审核通过的素材上传到 ImgAnt `6url` 相册并登记 asset，完成四语言内容和发布预检后 commit。
6. 广告上线事务同时写首页 revalidation 与通知 outbox；广告主收到包含展示期限和页面链接的上线邮件，管理员收到 Slack 通知。
7. 到期 Worker 把广告切换为 `expired` 并刷新首页；全额退款把已发布广告切换为 `canceled`，同时写首页 revalidation 与通知 outbox。

## 11. 反链检查设计

### 11.1 检查结果

每次检查记录：

- 最终 URL 与跳转链。
- HTTP 状态。
- Content-Type 和响应大小。
- 页面 canonical、robots meta。
- 匹配到的 href、anchor 和 rel。
- 延迟、错误类型和检查时间。
- DNS 解析结果的安全分类摘要。

结果状态：

- `healthy`
- `missing_link`
- `http_error`
- `timeout`
- `blocked`
- `invalid_content`
- `unsafe_target`

### 11.2 匹配规则

- 页面返回成功 HTML。
- backlink URL 的 registrable domain（基于 Public Suffix List 计算 eTLD+1）必须与申请网站的 canonical domain 一致。
- 跳转后的 registrable domain 继续保持一致；跨域跳转、集团其他域名和独立活动域名进入人工所有权审核。
- 页面包含指向 6URL canonical host `6url.net` 的可见 `<a>`；推荐目标为 `https://6url.net/`。
- href 完成协议、主机、端口、尾斜杠和追踪参数规范化后匹配。
- anchor 使用自然品牌文本；系统记录但不要求关键词。
- `nofollow`、`sponsored` 和普通链接均可作为合作资格，后台展示实际 rel。
- JavaScript 运行后才出现的链接首版列为人工复核，Worker 以原始 HTML 为主。

后台人工批准替代域名时记录申请 site、替代域名、证据、操作者和有效期。Worker 对跳转链中的每一跳继续执行 SSRF 校验，域名所有权批准不会放宽网络安全规则。

### 11.3 SSRF 防护

反链检查会访问用户提供的 URL，属于高风险网络入口。实现必须：

- 仅允许 `http` 和 `https`。
- 拒绝 URL 用户信息、非标准危险端口和超长地址。
- DNS 解析后拒绝 loopback、私网、链路本地、保留地址和云元数据地址。
- 自定义 dialer 固定到已验证公网地址，降低 DNS rebinding 风险。
- 每次重定向重新执行协议、端口、DNS 和 IP 校验。
- 限制跳转次数、响应体大小、总耗时和并发。
- 禁止把响应正文写入普通日志。
- 使用固定可识别 User-Agent，并提供联系页。

### 11.4 调度与误判控制

- 每日一次，按 placement ID 加入稳定随机抖动。
- Redis 租约确保多实例只执行一次。
- 失败后在 6～12 小时内补查一次。
- 需要连续 3 次失败且覆盖 72 小时才自动暂停。
- 管理员可标记维护窗口、手动重查和临时豁免。
- DNS、网络和 5xx 错误与明确缺失链接分别统计。

## 12. 数据模型

### 12.1 `locales`

```text
code primary key: zh-CN | en | ja | ko
url_prefix: "" | en | ja | ko
display_name
native_name
enabled
is_default
sort_order
created_at / updated_at
```

代码中维护同一份允许列表，数据库控制运营启停。首版种子固定创建四种 locale，并约束只能有一个默认语言。

### 12.2 `categories`

```text
id
parent_id nullable
slug unique
status: draft | published | archived
sort_order
created_at / updated_at
```

### 12.3 `category_content_revisions`

```text
id
category_id
locale
revision_no
name
description
intro_markdown
faq_json
seo_title
seo_description
source_locale nullable
source_revision_id nullable
created_by_token_id
content_updated_at
created_at
unique(category_id, locale, revision_no)
```

revision 行创建后保持不可变。每次保存生成下一 `revision_no`，审核状态保存在 locale publication 上，来源关系通过 `source_revision_id` 精确定位，避免 source locale 更新后无法判断译文基于哪个版本。

### 12.4 `category_locale_publications`

```text
category_id
locale
draft_revision_id nullable
draft_review_status: draft | in_review | needs_changes | approved | source_changed
published_revision_id nullable
version
published_at nullable
created_at / updated_at
unique(category_id, locale)
```

公开读取只连接 `published_revision_id`。编辑和 AI 更新只推进 `draft_revision_id`，正式发布在事务中把通过审核的 draft revision 切换为 published revision；旧发布内容在切换前保持不变。

### 12.5 `sites`

```text
id
slug unique
canonical_name
canonical_url
normalized_url
normalized_url_hash unique
canonical_domain
canonical_registrable_domain
status: draft | published | archived | removed
created_via: migration | submission | admin_manual
logo_asset_id nullable
facts_version
first_reviewed_at
last_verified_at
created_at / updated_at
```

`canonical_url` 表示稳定官方目标。商业关系、追踪 URL 和链接资格属于 placement，避免商业状态污染网站实体。共享事实变化时递增 `facts_version`，各 locale 内容据此判断事实是否需要复核。

MySQL 使用完整标准化 URL 的 SHA-256 作为唯一索引，完整 URL 继续保存在正文列中，避免 `utf8mb4 VARCHAR(2048)` 唯一索引超过 InnoDB 键长度上限。

`sites` 中的 canonical、domain、Logo 等字段代表当前公开共享事实。此类修改使用两阶段 preview / commit，在正式提交事务中更新并递增 `facts_version`；申请素材与待确认建议保留在 submission 或 operation payload 中，不通过普通草稿 PATCH 覆盖公开事实。

### 12.6 `site_categories`

```text
site_id
category_id
is_primary
sort_order
created_at / updated_at
unique(site_id, category_id)
```

服务层和数据库约束保证每个 site 最多一个 `is_primary=true`。该表是网站分类的唯一数据来源，Paid submission 与 Link exchange submission 都不能改变编辑分类。

编辑分类变更使用 version 与两阶段提交，并与 revalidation outbox 同事务生效。预览期间公开分类保持当前值，commit 后阻止旧版本请求覆盖新结果。

### 12.7 `site_content_revisions`

```text
id
site_id
locale
revision_no
name
tagline
summary
body_markdown
features_json
use_cases_json
pros_json
limitations_json
pricing_summary
pricing_checked_at nullable
seo_title
seo_description
source_locale nullable
source_revision_id nullable
source_facts_version
created_by_token_id
content_updated_at
created_at
unique(site_id, locale, revision_no)
```

revision 行创建后保持不可变。`source_revision_id` 可以指向任一 source locale 的 revision；`source_facts_version` 固定记录创建该版本时使用的共享事实版本。

### 12.8 `site_locale_publications`

```text
site_id
locale
draft_revision_id nullable
draft_review_status: draft | translation_requested | in_review | needs_changes | approved | source_changed
published_revision_id nullable
version
published_at nullable
created_at / updated_at
unique(site_id, locale)
```

内容页面是否公开由 `published_revision_id` 按 locale 决定。一个 site 可以同时拥有稳定发布的中文 revision、审核中英文 draft revision、待同步日文和缺失韩文。公开 API、HTML、sitemap、hreflang 与 feed 始终读取 published revision。

### 12.9 内容来源

```text
site_sources:
  id
  site_id
  locale nullable
  url
  title
  publisher
  source_type: official | documentation | pricing | independent
  checked_at
  created_at

site_revision_sources:
  site_content_revision_id
  site_source_id
  sort_order
  unique(site_content_revision_id, site_source_id)

category_sources:
  id
  category_id
  locale nullable
  url
  title
  publisher
  checked_at
  created_at

category_revision_sources:
  category_content_revision_id
  category_source_id
  sort_order
  unique(category_content_revision_id, category_source_id)
```

来源记录创建后保持不可变，复核或修正会创建新记录。revision 通过关联表固定当时使用的来源集合，因此编辑来源不会提前改变已发布页面。`locale` 为空表示跨语言来源，有值表示来源本身的主要语言。

### 12.10 `placements`

```text
id
site_id
kind: editorial | exchange | paid | affiliate | owned
origin: submission | admin | migration
status: pending | active | suspended | expired | canceled
submission_id nullable
payment_order_id nullable
outbound_url nullable
outbound_rel
disclosure_key
starts_at
ends_at nullable
backlink_url nullable
consecutive_failures
last_check_status nullable
last_checked_at nullable
suspended_reason
created_at / updated_at
```

一个 site 可以拥有多条历史 placement；`paid` 与 `exchange` 两类合计最多存在一条 active 记录，并通过事务完成两种申请方式之间的切换。公开收录申请只创建这两类 placement；`editorial`、`affiliate` 和 `owned` 仅用于内部编辑关系与存量数据标记。公开 CTA、链接属性和商业披露由服务层按 9.2 的固定优先级解析；没有有效 placement 时只展示经过编辑确认的实体内容与来源。

placement 不保存分类、广告位置或付费排序权重。分类完全来自 `site_categories`，首版所有申请收录使用同一标准展示规则。

`placement_locales` 保存 `(placement_id, locale)` 唯一组合。编辑收录可以覆盖所有已发布语言；付费和合作收录依据套餐或合作约定写入覆盖语言。

管理员免费直录创建 `kind=editorial`、`origin=admin` 且 order / submission 均为空的 placement。Paid submission 必须关联 paid order，Link exchange submission 必须关联 submission 与 backlink；数据库约束与 service 预检阻止来源关系不完整。

### 12.11 Sponsor 广告

```text
ads:
  id
  site_id nullable
  ad_application_id unique nullable
  payment_order_id unique nullable
  status: draft | active | paused | expired | canceled
  target_url
  image_asset_id
  starts_at nullable
  ends_at nullable
  sort_order
  origin: admin | advertiser | migration
  created_by_token_id
  version
  created_at / updated_at

ad_localizations:
  ad_id
  locale
  title
  description
  cta_label
  alt_text
  version
  created_at / updated_at
  unique(ad_id, locale)
```

广告是独立实体，`site_id` 只提供可选关联。`origin=advertiser` 必须关联已审核广告申请和已付款广告订单；`origin=admin` 与 `origin=migration` 的申请及订单关联必须为空。active 广告在当前 locale 存在完整 localization 时才进入对应语言首页；缺少语言时不回退其他语言文案。所有广告写操作使用 preview / commit、ETag 与审计日志，commit 与首页 revalidation job 同事务提交。前端从广告查询服务固定获得 `rel="sponsored noopener"` 和可见广告标识。

### 12.12 `ad_applications`

```text
id
public_token_hash unique
status: received | needs_changes | preapproved | awaiting_payment | paid | published | rejected | canceled
applicant_email
applicant_name
preferred_locale: zh-CN | en | ja | ko
target_url
requested_locales_json
raw_title
raw_description
source_image_url nullable
image_asset_id nullable
reviewer_token_id nullable
review_note
created_at / updated_at
```

广告申请使用独立状态机和状态页面。公开目录 submission 不允许转成广告申请，广告申请也不创建 site placement。状态 token、短期 HttpOnly session、邮件与 Slack outbox 沿用目录申请的安全机制。

### 12.13 `submissions`

```text
id
public_token_hash unique
type: exchange | paid
status
applicant_email
applicant_name
preferred_locale: zh-CN | en | ja | ko
raw_name
raw_url
normalized_url
normalized_registrable_domain
raw_description
requested_category_id
backlink_url nullable
duplicate_site_id nullable
reviewer_token_id nullable
review_note
submitted_at / reviewed_at / created_at / updated_at
```

数据库只保存 public token 的 hash。邮件中的原始 token 只展示给申请人。

### 12.14 `application_sessions`

```text
id
subject_type: submission | ad_application
submission_id nullable
ad_application_id nullable
session_hash unique
csrf_secret_hash
expires_at
last_seen_at
revoked_at nullable
created_at / updated_at
```

目录申请和广告申请各自生成 public token；session 行必须且只能关联其中一种申请。原始 token 只用于创建短期 session。浏览器持有 HttpOnly cookie，数据库保存 session hash；退出、重新生成 public token、过期和申请关闭均可撤销 session。

### 12.15 `listing_plans`

```text
id
code unique
name
amount_minor
currency
duration_days nullable
included_locales_json
stripe_price_id unique nullable
active
sort_order
created_at / updated_at
```

首版只创建一个 active plan，公开提交页直接展示该套餐。保留 `listing_plans` 表用于价格和权益快照、停用旧版本及未来扩展；简单后台首版不提供多套餐配置功能。

首版 active plan 的 `currency` 固定为小写 ISO 4217 值 `usd`，页面明确展示 `$` 与 `USD`，避免不同 locale 对结算币种产生误解。

### 12.16 `ad_offers`

```text
id
code unique
name
amount_minor
currency
duration_days
included_locales_json
stripe_price_id unique nullable
active
created_at / updated_at
```

广告产品与收录套餐独立配置。首版广告产品数量、USD 价格、展示期限和 locale 覆盖仍待确认；订单保存广告权益快照，后续改价不会影响历史广告。

### 12.17 `payment_orders`

```text
id
purpose: listing | ad
submission_id nullable
ad_application_id nullable
site_id nullable
listing_plan_id nullable
ad_offer_id nullable
status: pending | processing | paid | failed | expired | refunded | pending_manual
provider: stripe | manual
amount_minor
currency
livemode
duration_days_snapshot nullable
locales_snapshot_json
stripe_price_id_snapshot nullable
stripe_checkout_session_id unique nullable
stripe_payment_intent_id unique nullable
external_ref unique nullable
paid_at / refunded_at nullable
created_at / updated_at
```

`purpose=listing` 的订单必须关联 Paid submission 与 listing plan；`purpose=ad` 的订单必须关联广告申请与 ad offer。两类订单共享 Stripe 事件处理和财务审计，同时通过数据库 XOR 约束和领域服务保持权益隔离。Stripe 订单金额使用 USD 最小货币单位（美分），订单保存权益快照，后续修改产品不会重写历史订单。Stripe webhook 中出现其他币种时拒绝激活权益并产生 Slack 告警。

首版只执行全额退款，因此 `refunded` 是终态。异步支付先进入 `processing`；provider 明确确认 paid 后，listing 订单激活 placement，广告订单进入待发布广告流程。过期 Checkout Session 进入 `expired`，允许管理员按规则创建新订单。

### 12.18 `payment_events`

参考 4aicode 的 Stripe 幂等事件表：

```text
id
provider
event_id unique
type
payload_encrypted_or_redacted
received_at
processing_started_at nullable
processed_at nullable
error_summary
```

### 12.19 `backlink_checks`

```text
id
placement_id
status
requested_url
final_url
requested_registrable_domain
final_registrable_domain
domain_match_status
http_status nullable
matched_href nullable
matched_anchor nullable
matched_rel nullable
latency_ms
error_code nullable
checked_at
```

按 `(placement_id, checked_at)` 建索引，并设置历史保留策略。

### 12.20 `assets`

```text
id
provider: imgant
provider_image_id unique
storage_key unique
public_url
origin_url
album_id
album_name
content_hash nullable
content_type
size_bytes
width nullable
height nullable
source_url nullable
status: active | unhealthy | pending_delete | deleted
deleted_at nullable
created_at / updated_at
```

`album_name` 首版固定为 `6url`。数据库保存公开资源标识，不保存 ImgAnt API Key 和 delete token。

asset 删除前必须查询 site、所有 content revision、locale publication 和 ads 引用。存在引用时返回 `409 asset_in_use`；ImgAnt 删除或探测失败时保留数据库记录并标记 `unhealthy` / `pending_delete`，阻断新发布，同时让现有页面使用受控占位图并产生告警。

### 12.21 `admin_api_tokens`

```text
id
name
token_prefix
token_hash unique
scopes_json
last_used_at nullable
expires_at nullable
revoked_at nullable
created_at / updated_at
```

原始 token 由后端管理 CLI 生成并只显示一次。请求鉴权后把 token ID 写入 context，后续审计与限流均使用该 ID。

### 12.22 `admin_sessions`

```text
id
admin_api_token_id
session_hash unique
csrf_secret_hash
expires_at
last_seen_at
revoked_at nullable
created_at / updated_at
```

浏览器只持有随机 session 原文和 CSRF token，数据库保存 hash。每次请求同时检查 session 与关联管理 token 的有效状态，保证 token 撤销可以立即终止后台会话。

### 12.23 `notification_deliveries`

```text
id
kind
channel: email | slack
recipient
locale nullable
template_key
template_version
payload_encrypted
status: pending | sending | sent | failed
dedupe_key unique nullable
attempts
max_attempts default 5
next_retry_at
last_error nullable
provider_message_id nullable
sent_at nullable
created_at / updated_at
```

业务事务只写 notification outbox，Worker 提交后渲染模板并投递。包含申请状态 token 的 payload 使用服务端加密密钥加密，投递成功或达到保留期限后清除密文；普通日志只记录 delivery ID、kind、template、locale 和脱敏收件人。

### 12.24 辅助表

- `redirects`：旧 path、新 path、状态码、创建原因。
- `content_change_events`：记录 revision 指针变化、操作 token、diff 摘要和发布时间，正文历史由各 revision 表承担。
- `admin_audit_logs`：关键管理操作。
- `revalidation_jobs`：与发布状态同事务写入的 outbox，保存 Next.js 刷新事件、抢占时间、重试次数和完成时间。

## 13. API 设计

### 13.1 公开读取

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/locales` | 启用的语言列表 |
| `GET` | `/api/v1/catalog?locale=zh-CN` | 对应语言首页和机器可读目录 |
| `GET` | `/api/v1/categories?locale=zh-CN` | 对应语言已发布分类 |
| `GET` | `/api/v1/categories/:slug/sites?locale=zh-CN` | 对应语言分类网站列表 |
| `GET` | `/api/v1/sites/:slug?locale=zh-CN` | 网站实体、对应语言内容和来源 |
| `GET` | `/api/v1/updates?locale=zh-CN` | 对应语言最近实质更新 |

公开 API 只接受允许列表中的 locale，缺少参数时使用 `zh-CN`。响应返回 `locale`、`available_locales`、`updated_at`、稳定实体 ID 和内容版本，使用 CDN、ETag 和合理速率限制。

### 13.2 申请与支付

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/submissions` | 创建申请并保存 preferred locale |
| `POST` | `/api/v1/submission-session` | 用 fragment 中的原始 token 换取短期 HttpOnly session |
| `DELETE` | `/api/v1/submission-session` | 撤销当前申请 session |
| `GET` | `/api/v1/submissions/me` | 使用申请 session 查询状态 |
| `POST` | `/api/v1/submissions/me/recheck` | 使用申请 session 请求重查合作链接 |
| `POST` | `/api/v1/submissions/me/checkout` | 使用申请 session 创建已预审订单的 Checkout |
| `POST` | `/api/v1/ad-applications` | 创建独立 Sponsor 广告申请 |
| `POST` | `/api/v1/ad-application-session` | 用广告状态 token 换取短期 HttpOnly session |
| `DELETE` | `/api/v1/ad-application-session` | 撤销当前广告申请 session |
| `GET` | `/api/v1/ad-applications/me` | 查询当前广告申请、审核、支付与展示状态 |
| `POST` | `/api/v1/ad-applications/me/checkout` | 为已审核广告申请创建 Stripe Checkout |
| `POST` | `/api/v1/webhooks/stripe` | Stripe webhook |

状态接口只返回申请人所需字段。目录申请与广告申请使用不同 cookie 名称和 audience，防止 session 跨业务复用。session 交换按 IP、申请 ID 和失败次数严格限流，请求 body 禁止进入 access log、应用日志、APM 和错误追踪；写操作同时校验 Origin 与 CSRF token。

### 13.3 管理 API

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST/DELETE` | `/admin/v1/session` | token 换取后台会话 / 退出 |
| `GET` | `/admin/v1/me` | 当前 token、scope 和 session 信息 |
| `GET` | `/admin/v1/overview` | 后台关键数量与异常摘要 |
| `GET` | `/admin/v1/submissions` | 申请队列与筛选 |
| `POST` | `/admin/v1/submissions/:id/preapprove` | 通过预审 |
| `POST` | `/admin/v1/submissions/:id/request-changes` | 要求补充 |
| `POST` | `/admin/v1/submissions/:id/reject` | 拒绝 |
| `GET` | `/admin/v1/ad-applications` | Sponsor 广告申请队列与筛选 |
| `POST` | `/admin/v1/ad-applications/:id/preapprove` | 通过广告预审并准备广告订单 |
| `POST` | `/admin/v1/ad-applications/:id/request-changes` | 要求广告主补充或修改材料 |
| `POST` | `/admin/v1/ad-applications/:id/reject` | 拒绝广告申请 |
| `GET/POST/PATCH` | `/admin/v1/sites/*` | 网站内容管理 |
| `POST` | `/admin/v1/sites/manual` | 管理员免费直录，创建 site 草稿与 editorial placement |
| `POST` | `/admin/v1/sites/:id/publish` | 发布并触发刷新 |
| `GET/POST/PATCH` | `/admin/v1/categories/*` | 分类管理 |
| `GET/POST/PATCH` | `/admin/v1/placements/*` | 收录资格管理 |
| `GET/POST/PATCH` | `/admin/v1/ads/*` | Sponsor 广告查看、创建与编辑 |
| `POST` | `/admin/v1/ads/:id/preview` | 预检广告 locale、素材、目标 URL、标识和首页变化 |
| `POST` | `/admin/v1/ads/:id/commit` | 启停、排序或发布广告并触发首页刷新 |
| `GET` | `/admin/v1/backlink-checks` | 异常和历史 |
| `POST` | `/admin/v1/placements/:id/recheck` | 手动重查 |
| `GET` | `/admin/v1/orders` | 订单列表 |
| `POST` | `/admin/v1/orders/:id/mark-paid` | 人工确认付款 |
| `POST` | `/admin/v1/orders/:id/refund` | 退款 |
| `GET` | `/admin/v1/seo/health` | 索引发布门槛与链接健康 |
| `GET` | `/admin/v1/i18n/health` | 四语言缺失、版本差异和 hreflang 健康 |
| `GET` | `/admin/v1/jobs` | revalidation 与通知任务 |
| `POST` | `/admin/v1/jobs/:id/retry` | 重试失败任务 |
| `GET` | `/admin/v1/audit-logs` | AI、CLI 与后台审计记录 |
| `POST` | `/admin/v1/assets/register` | 登记并验证 ImgAnt `6url` 相册图片 |
| `POST` | `/admin/v1/assets/:id/delete/preview` | 检查所有引用并预览图片删除 |
| `POST` | `/admin/v1/assets/:id/delete/commit` | 再次检查引用并原子标记 `pending_delete`，阻止新引用 |
| `POST` | `/admin/v1/assets/:id/delete/result` | 幂等登记本地 AI 执行的 ImgAnt 删除结果 |
| `POST` | `/admin/v1/sites/:id/publish/preview` | 运行发布预检并返回短时 operation token |
| `GET` | `/admin/v1/openapi.json` | AI 可读取的 OpenAPI 3.1 描述 |

所有管理写操作记录 token ID、目标、变更摘要、request ID 和 idempotency key。资源读响应返回 `ETag` / `version`，后台 UI 与本地 AI 的更新请求均携带 `If-Match`，防止并发覆盖。

## 14. 简单管理后台与本地 AI

管理后台和本地 AI 共用 `/admin/v1`。后台提供可视化观察与快捷操作，本地 AI 负责批量编辑、四语言内容生产、来源补全和复杂流程编排。所有状态转换、校验、审计和幂等逻辑都在 Go service 与管理 API 中执行。

### 14.1 简单管理后台范围

首版页面：

- `/admin`：关键数量、待处理申请、反链异常、即将到期、刷新失败。
- `/admin/submissions`：申请列表、详情和状态时间线。
- `/admin/sites`：网站、四语言发布状态、placement、页面预览链接。
- `/admin/ads`：用户广告申请队列、4 个现有 Sponsor 广告、locale 文案、素材、排序、状态和有效期。
- `/admin/orders`：订单、Stripe event 和人工订单状态。
- `/admin/backlinks`：反链健康、连续失败、暂停原因和最近检查。
- `/admin/jobs`：revalidation 与通知失败任务。
- `/admin/audit`：AI、CLI 和后台操作审计。

首版快捷操作：

- 申请预审通过、要求补充、拒绝。
- 手动触发反链重查。
- placement 暂停与恢复。
- 失败 revalidation 重试。
- 网站和四语言页面预览。
- 管理员免费直录网站；Sponsor 广告申请审核、免费创建、编辑、排序、启停和预览。
- 人工付款、退款、发布和永久删除继续使用两阶段预览与确认。

首版后台范围保持精简：

- 结构化数据以表格、详情和 JSON diff 展示。
- 内容编辑提供基础字段和 Markdown 文本框。
- 批量翻译、批量改写、来源研究和批量导入由本地 AI 完成。
- 套餐配置、复杂报表和拖拽式页面搭建进入后续范围。

后台使用现有 Next.js 应用的 `/admin` 路由，Tailwind CSS、Radix UI 和 TanStack Query 复用 4aicode 的前端模式。UI 只调用管理 API，不直接访问数据库。

### 14.2 后台会话鉴权

浏览器后台使用短期 HttpOnly session：

1. 管理员在 `/admin/login` 输入本地保存的管理 token。
2. `POST /admin/v1/session` 验证 token hash、scope、到期和撤销状态。
3. 服务端创建 `admin_sessions`，设置 `HttpOnly; Secure; SameSite=Strict` 的随机 session cookie。
4. 浏览器后续调用管理 API 使用 session cookie；本地 AI 继续使用 Bearer token。
5. session 默认 8 小时，退出、token 撤销和超时均立即失效。

后台写操作校验 Origin 与 CSRF token。session 继承原 token scope，后台按钮根据 scope 隐藏或禁用；服务端继续执行最终授权。

`POST /admin/v1/session` 使用独立的严格 IP / token-prefix 速率限制和递增退避，请求 body 中的原始 token 从 access log、应用日志、APM 与错误追踪中排除。登录成功后轮换 session ID。

生产环境前台与后台页面使用 `https://6url.net` 和 `https://6url.net/admin`，Go API 使用 `https://api.6url.net`。管理 API CORS 只允许精确 origin `https://6url.net` 并开启 credential。session cookie 绑定 `api.6url.net`，Next.js 后台请求显式使用 `credentials: include`。后台响应配置严格 CSP，至少限制 `default-src 'self'`、`frame-ancestors 'none'`、`base-uri 'none'` 和受控的 ImgAnt 图片 host。

### 14.3 本地 AI 配置

建议的本地配置项：

```dotenv
SIXURL_ADMIN_API_URL=https://api.6url.net
SIXURL_ADMIN_API_TOKEN=<只在本地保存的原始 token>
IMGANT_API_KEY=<ImgAnt 本地 API Key>
IMGANT_ENDPOINT=https://api.imgant.com
IMGANT_ALBUM=6url
```

- 原始 token 放在仓库外的本地私有配置或系统钥匙串，文件权限设为 `0600`。
- Git 仓库只提交变量名示例，真实 token 保持在版本控制之外。
- AI 工具从环境读取 token，日志和命令输出只展示 token prefix。
- 服务端数据库只保存 token hash、prefix、名称、scope、最后使用时间和撤销时间。
- token 可以独立撤销与轮换，服务端使用常量时间比较。
- 仓库维护完整的项目根目录或 `backend/.env.example`、必填项说明和启动时校验；生产环境变量与真实密钥由项目所有者配置。

首个 token 通过服务器本地 CLI 创建，例如：

```text
6url-admin tokens create \
  --name local-ai-content \
  --scopes catalog:read,content:write,content:publish,assets:write,submissions:review,ad_applications:review,placements:write,ads:write
```

CLI 只显示一次原始 token，数据库保存 hash。token 撤销和轮换继续通过服务器 CLI 完成，保证管理 API 无需预置万能 bootstrap token。

### 14.4 Token scope

| Scope | 能力 |
|---|---|
| `catalog:read` | 读取分类、网站、版本和 SEO 健康 |
| `content:write` | 编辑草稿、来源和四语言内容 |
| `content:publish` | 发布、归档和重定向 |
| `assets:write` | 登记已上传到 ImgAnt `6url` 相册的图片 |
| `submissions:review` | 审核、补充、拒绝申请 |
| `ad_applications:review` | 审核 Sponsor 广告申请并准备广告订单 |
| `placements:write` | 激活、暂停与恢复收录资格 |
| `ads:write` | 管理 Sponsor 广告、locale 文案、排序与启停 |
| `payments:read` | 查看订单与 webhook 状态 |
| `payments:write` | 人工确认付款和退款 |
| `system:revalidate` | 重试页面刷新任务 |

日常 AI token 建议包含读取、内容、图片登记、收录审核、广告审核、placement 和 ads scope。人工付款与退款使用单独的高权限 token，降低普通内容操作凭证泄漏后的影响范围。

### 14.5 AI 友好 API 约定

- OpenAPI 3.1 描述完整请求、响应、枚举、scope 和示例。
- 列表接口支持稳定分页、过滤、排序和 `updated_since`。
- 错误统一返回 `{code, message, details, request_id}`。
- 资源响应包含 `version` 和 `updated_at`。
- 更新请求使用 `If-Match` 或显式 version，冲突返回 `409 version_conflict`。
- 写操作支持 `Idempotency-Key`，服务端保存操作结果并安全重放。
- 批量操作返回逐项结果，避免部分失败丢失信息。
- 所有时间使用 RFC 3339 UTC，金额使用最小货币单位。
- AI 可以先调用预览接口获取 diff、警告和阻断项，再提交正式操作。

### 14.6 高影响操作

发布、永久删除、人工确认付款和退款采用两阶段调用：

1. `preview` 接口执行完整校验，返回 diff、风险、目标 version 和 10 分钟有效的 `operation_token`。
2. `commit` 接口提交 operation token、幂等键和操作原因。
3. 服务端校验资源 version 未变化后执行事务。
4. 结果写入审计日志，返回受影响页面和 revalidation job ID。

永久删除继续遵循 301 / 410 规则。普通草稿编辑保持单阶段 API，并通过 version 控制并发覆盖。

### 14.7 发布预检

阻断项：

- canonical 或 normalized URL 冲突。
- slug 冲突。
- 内容审核未通过。
- 当前 locale 内容缺失、语言检测不匹配或源版本过期。
- 缺少来源或关系披露。
- 商业关系与链接属性不一致。
- Logo 尚未登记为 ImgAnt `6url` 相册资产。
- JSON-LD 生成失败。

提示项：

- title 或 description 与站内高度相似。
- 定价信息超过设定更新时间。
- 正文信息密度不足。
- 相关网站和内部链接不足。
- 四种语言之间的事实字段存在待确认差异。

### 14.8 本地 AI 操作流程

```text
读取 OpenAPI / 健康状态
        ↓
查询待处理申请或内容差异
        ↓
读取资源当前 version
        ↓
编辑草稿与来源
        ↓
调用 publish preview
        ↓
人工确认高影响操作
        ↓
携带 operation token + Idempotency-Key 提交
        ↓
读取审计结果和页面刷新状态
```

本地提供一个薄 CLI 封装认证头、幂等键、分页和 JSON 输出。AI 优先调用 CLI，OpenAPI 作为接口发现与参数依据。

## 15. 技术架构

```text
访问者 / 搜索爬虫 / AI 搜索
              │
              ▼
      Next.js SEO Frontend
      SSG / ISR / JSON-LD
              │ server-side fetch
              ▼
        Go API (Gin)
  ┌───────────┴─────────────┐
  ▼                         ▼
MySQL                     Redis
  │                         │
  ▼                         ▼
Ent ORM              Worker lease / rate limit
              │
       ┌──────┴────────┐
       ▼               ▼
 Backlink Checker   Email / Slack Notifications

Stripe ── signed webhook ──► Go API
Local AI / Admin CLI ── scoped token ──► Go API
Local AI / imgant CLI ── API Key ─────► ImgAnt `6url` album
Next.js ── public responsive images ───► ImgAnt
```

### 15.1 前端

- Next.js 当前稳定版。
- App Router 或等价服务端数据能力。
- 默认中文无前缀路由，`en`、`ja`、`ko` 使用前缀路由。
- locale 配置集中管理 URL、BCP 47 code、Open Graph locale 和日期数字格式。
- 主体内容使用 Server Components / 静态 HTML。
- Sponsor 广告模块使用服务端 HTML，固定输出可见广告标识与 `sponsored noopener`，并按 locale 过滤完整广告文案。
- Tailwind CSS + Radix UI 可复用 4aicode 的设计基础。
- 前台页面避免依赖客户端全局状态。

### 15.2 后端

- Go 1.23+、Gin、Ent、MySQL 8。
- 复用 4aicode 的配置、日志、API key 鉴权、Stripe 事件幂等、邮件和 Worker 结构。
- 邮件直接移植 4aicode 的 `internal/mailer` 接口与 UnoSend / Sendflare provider 适配层，通知任务移植 `internal/notifications` 的 outbox Worker 模式。
- 业务按 handler / service / repository 分层。
- 所有状态转换集中到 service，避免后台、webhook 和 Worker 各自实现不同规则。
- 所有公开内容查询显式携带 locale，并只返回该 locale 已发布版本。
- 翻译状态与事实源版本在 service 层统一推进。

### 15.3 Redis

- 提交与状态接口限流。
- 反链 Worker 分布式租约。
- 短期 API 缓存和防重复通知。
- Redis 故障时数据库仍是状态真相；关键订单和发布流程保持可恢复。

### 15.4 ImgAnt 图片托管

首版统一把 Logo 和截图上传到 ImgAnt 的 `6url` 相册。已核对的 API 契约：

```http
POST https://api.imgant.com/api/v1/api-upload
X-API-Key: <IMGANT_API_KEY>
Content-Type: multipart/form-data

file=@logo.png
album=6url
```

成功响应为 HTTP 201，关键字段包括：

```json
{
  "success": true,
  "data": {
    "id": "storage-key",
    "link": "https://cdn.imgant.com/..."
  },
  "image": {
    "id": 123,
    "url": "https://cdn.imgant.com/...",
    "origin_url": "https://cdn.imgant.com/...",
    "album": { "id": 42, "name": "6url" }
  }
}
```

ImgAnt 支持 `GET /api/v1/api-albums` 查询 API Key 可访问的相册。按相册名称上传时匹配不区分大小写，并要求可访问范围内名称唯一。上线前验证 `6url` 相册唯一且 API Key 具有上传权限。

首版本地 AI 流程：

1. 安全下载或读取待上传图片。
2. 校验 magic bytes、文件大小、像素数量和格式，并在本地重编码以清除 metadata。
3. 使用 `imgant upload <file> --album "6url" --json` 或等价 HTTP API 上传。
4. 调用 6URL `POST /admin/v1/assets/register` 登记 ImgAnt 返回数据。
5. 6URL 校验 `image.album.name` 为 `6url`、URL host 在允许列表内，并按内容 hash 去重。
6. site 只引用内部 `asset_id`，公开 API 输出已登记的 ImgAnt URL。

`/admin/v1/assets/register` 属于受信任的管理操作，首版对相册信息进行结构校验和公开 URL 探测。6URL 服务端不持有 ImgAnt 凭证，相册归属验证属于结构性验证；信任边界由 `assets:write` scoped token、精确 host 允许列表和审计日志承担。

本地 AI 持有 ImgAnt API Key，6URL 服务端首版只保存公开资源标识和 URL。后续自动化导入可以为 6URL Worker 配置独立、可撤销的 ImgAnt API Key，上传逻辑固定 `album=6url`。ImgAnt 提供相册级 scope 后再进一步收窄权限。

站点归档时保留已引用图片。图片删除作为独立高影响操作：preview 检查所有 site、revision 和 locale publication 引用；commit 再次检查并原子标记 `pending_delete`，此后数据库禁止建立新引用；本地 AI 再使用 ImgAnt 凭证执行远端删除并通过 result 接口登记结果。存在引用时返回 `409 asset_in_use`，远端失败时资产保持 `pending_delete` 并进入重试 / 人工处理。探测到外部丢失时标记 `unhealthy`、阻断新发布并让现有页面回退到受控占位图。

Next.js 使用 ImgAnt 允许的图片 host 和图片变换参数生成响应式资源。ImgAnt 已支持 `format`、`quality`、`width`、`height` 等变换参数；前端 loader 只生成受控参数，原始 URL 作为回退。

### 15.5 部署

- `https://6url.net` 部署 Next.js 到 Vercel，包含公开页面与 `/admin`，保留 ISR 和按需刷新能力。
- `https://api.6url.net` 部署 Go API，使用 4aicode 的 systemd + nginx 模式。
- `www.6url.net` 与 HTTP 入口单跳 301 到 `https://6url.net` 的对应路径。
- MySQL、Redis 和备份沿用现有运维经验。
- 建立 development、staging、production 三套配置。
- 仓库提供配置示例和启动校验，生产环境变量、邮件凭证、Slack 凭证与支付密钥由项目所有者配置。

## 16. 安全与隐私

### 16.1 公共输入

- Turnstile、人机验证和 IP / 邮箱 / 域名多维限流。
- URL、邮箱、文本长度和字符集校验。
- Markdown 在服务端转换并使用允许列表清理 HTML。
- 上传文件校验 magic bytes、尺寸和像素数量。
- 广告目标 URL 与原始素材 URL 使用与反链 Checker 等价的 SSRF、重定向和响应大小限制；后台预览不得由浏览器直接请求任意源站。
- 提交内容只进入后台，审核通过后才公开。
- 申请状态 token 只通过 URL fragment 到达客户端并换取短期 HttpOnly session；原始 token 和交换请求 body 禁止记录。

### 16.2 管理员

- scoped bearer token 只存 hash，支持到期、撤销和轮换。
- 管理 API 强制 HTTPS，并对 token ID 和来源 IP 限流。
- session 交换采用独立严格限流；CORS 使用精确后台 origin，后台页面启用严格 CSP。
- 内容 token 与支付写 token 分离。
- 高影响操作要求 operation token、资源 version、幂等键和操作原因。
- 后台和本地 AI 的更新均使用 ETag / `If-Match`，冲突返回 409。
- 退款、人工确认付款、发布、归档和 token 管理记录安全事件。

### 16.3 支付

- Stripe webhook 使用签名验证。
- 共享 Stripe 账号为 6URL 配置独立 webhook endpoint 与 signing secret，并只订阅 Checkout 生命周期和退款事件。
- event ID 唯一约束保证幂等。
- order 状态转换使用事务和条件更新。
- webhook 使用 `sixurl_app=6url.net`、`sixurl_order_id` 和本地 Session / PaymentIntent 隔离其他产品，再校验订单金额、币种、livemode 与支付对象归属，并覆盖同步、异步、过期和退款事件。
- 其他产品和不支持的 Stripe 事件返回 200 ignored，不写 `payment_events`，避免共享账号产生重试与数据污染。
- paid 状态是 placement 激活前置条件；首版退款只接受全额退款。
- 日志记录订单 ID 和事件 ID，不记录密钥和完整支付资料。
- 退款和人工付款需要操作原因。

### 16.4 隐私

- 申请邮箱只用于审核、付款和状态通知。
- 公开页面不展示申请人邮箱。
- 定义拒绝申请和过期申请的保留期限。
- 邮件 token 只保存 hash，可撤销并重新生成；关联 application session 同步撤销。
- token 邮件所需的原始值只短暂存在于内存和加密 notification payload，投递完成或超过保留期限后清除密文。

### 16.5 ImgAnt

- ImgAnt API Key 保存在本地 AI 私有配置中，只向 `api.imgant.com` 发送。
- 6URL 管理 API 不接收或保存 ImgAnt API Key。
- `/admin/v1/assets/register` 只接受 ImgAnt 允许 host、`album.name=6url` 和完整资源标识。
- 数据库不保存 ImgAnt `deletehash` / delete token。
- 图片下载阶段复用公网 IP 校验和大小限制，防止通过 Logo URL 访问内网。
- 上传前重编码，公开页面只使用已登记 asset。

## 17. 通知

| 事件 | 申请人 | 管理员 |
|---|---|---|
| 新申请 | 确认邮件与状态链接 | Slack |
| 要求补充 | 邮件 | 审计记录 |
| 预审通过 | 付款或合作链接指引 | 可选 Slack |
| 支付成功 | 收据与上线状态 | 可选 Slack |
| 发布成功 | 必发上线邮件、已发布 locale 与页面 URL | Slack |
| 反链首次失败 | 状态提示 | 异常列表 |
| 达到暂停阈值 | 暂停原因与恢复方法 | Slack |
| 即将到期 | 30 / 7 / 1 天提醒 | 到期列表 |
| 退款 | 退款确认邮件 | Slack 强通知 |

正式发布事务同时创建 notification delivery outbox。Worker 在提交后发送申请人邮件和管理员 Slack 通知；失败使用幂等键重试，邮件持续失败时发送 Slack 告警。通知系统故障不会回滚已完成的数据库发布，后台明确展示待发送和失败状态。

申请人通知根据 `preferred_locale` 使用中文、英文、日文或韩文模板。模板 key 和变量结构在四种语言间保持一致，缺少模板会阻断发布流程中的通知发送并产生管理员告警。

### 17.1 邮件实现复用 4aicode

邮件层参考 `/Users/forecho/Code/go/4aicode/backend/internal/mailer`：

- 复用 `Message`、`Sender`、`Config`、`NewSender` 和 `Service` 的分层方式，业务服务只调用具备语义的邮件方法。
- 沿用 UnoSend 默认适配器与 Sendflare 备选适配器，使用 Bearer API Key、发送地址、可覆盖 API base URL 和 10 秒请求超时。
- 每封申请人邮件同时生成纯文本与品牌 HTML，模板使用 6URL 品牌、`https://6url.net` 链接和当前 locale 文案。
- 复用 4aicode 的 branded email 组件思路，对所有动态文本执行 HTML escape，并保留纯文本 fallback。
- 状态链接和支付链接关闭供应商点击重写 / 跟踪，保证 fragment token 与 canonical URL 稳定。

投递层参考 `/Users/forecho/Code/go/4aicode/backend/internal/notifications` 与 `notification_event` Ent schema：

- 状态采用 `pending → sending → sent / failed`，使用唯一 `dedupe_key` 防止重复邮件。
- Worker 条件更新抢占任务，每 10 秒扫描一批；`sending` 超过 10 分钟自动回到 `pending`。
- 默认最多尝试 5 次，按 1、2、4、8、16 分钟指数退避；缺少线上邮件配置时保留 pending 并每 5 分钟复查，且不消耗尝试次数。
- 管理员 Slack 继续复用同一 outbox Worker，并通过 Slack SDK `chat.postMessage` 投递。
- 6URL 的生产邮件全部通过 outbox 发送，HTTP handler 与发布 service 不直接调用外部邮件 API。

6URL 在复用基础上增加 `locale`、`template_key`、`template_version`、加密 payload 和 HTML 渲染。新申请确认邮件的状态 token 只存在于内存和加密 outbox payload 中，数据库中的 submission 继续只保存 token hash。

实施时沿用 4aicode 的配置名称：`EMAIL_PROVIDER`、`EMAIL_API_KEY`、`EMAIL_FROM_ADDRESS`、`EMAIL_API_BASE_URL`、`APP_BASE_URL`、`ADMIN_NOTIFY_CHANNELS`、`SLACK_BOT_TOKEN`、`SLACK_CHANNEL_ID`。`APP_BASE_URL` 线上值为 `https://6url.net`，管理员通道配置为 Slack。仓库在引入这些配置的同一提交中补齐 `.env.example`，真实值由项目所有者部署时填写。

## 18. 存量数据迁移

### 18.1 导入流程

1. 读取迁移后的 `data/site.json` 和 `data/resources.json`；这两份文件原位于 `pages/api/`。
2. 按最终跳转结果和 canonical 域名规范化 URL。
3. 创建 categories。
4. 创建 `zh-CN` category / site content revision，并把 locale draft pointer 指向导入版本。
5. 创建 sites 和初始 editorial placement。
6. 通过 ImgAnt 上传到 `6url` 相册并生成 assets。
7. 识别重定向、联盟链接和重复域名，进入人工复核队列。
8. 保留当前首页分类顺序和卡片顺序。
9. 首页切换到 API / ISR 后，对比新旧输出。
10. 中文内容通过后，以 `zh-CN` revision 作为本次迁移的 source locale 创建英文、日文和韩文翻译任务。
11. 从实际生产配置或生产数据导出中取得现有 4 个 Sponsor 广告的名称、目标 URL、图片、排序、状态、有效期和 locale 文案。
12. 将确认后的 4 个广告导入 `ads` 与 `ad_localizations`，并逐语言对比迁移前后的首页广告输出。

源码盘点时只在 `pages/components/app.tsx` 找到 GitHub Sponsor 按钮，原 `pages/api/resources.json` 保存 6 个普通分类。实施阶段已分别迁移到 `components/app.tsx` 与 `data/resources.json`。线上 4 个广告的完整配置未进入当前版本库，因此迁移以实际生产配置或导出数据为唯一依据。

### 18.2 迁移质量策略

- 原卡片继续在首页展示，避免上线时内容骤减。
- 自动生成的详情页保持 `noindex`。
- 中文编辑逐条补充独立内容和来源，通过发布门槛后进入中文 sitemap。
- 英文、日文和韩文分别翻译、审核和发布，逐语言加入 hreflang 与 sitemap。
- 原有首页锚点继续工作，必要时添加兼容 ID。
- 原有出站 URL 保留追踪需求，同时补充 canonical target 和商业关系。
- 广告迁移逐项核对目标 URL、ImgAnt asset、四语言文案、排序、启停状态和可选有效期；缺失 locale 的广告仅从对应语言首页隐藏。
- 迁移验收保存前后首页广告快照，确保 4 个广告的身份、链接、素材、顺序和披露一致。

### 18.3 回滚

- 数据库上线初期保留 JSON 只读快照。
- 前端配置支持暂时读取最后一次成功 catalog 快照。
- 数据库迁移采用新增表和新增字段。
- 发布切换以 feature flag 控制数据源。
- 回滚前端不会删除数据库数据或支付记录。

## 19. 可观测性与运营指标

### 19.1 技术指标

- API 成功率、延迟和缓存命中率。
- ISR 刷新成功率与队列积压。
- Stripe webhook 成功率、重试和未处理事件。
- 反链检查成功率、错误分类、延迟和暂停数量。
- 邮件发送成功率。
- Slack 通知成功率、重试次数和失败积压。
- sitemap URL 数量、生成时长和状态分布。

### 19.2 SEO 指标

- Search Console 已发现、已抓取、已索引页面数量。
- 按 locale 统计已发布、已抓取、已索引页面和搜索表现。
- canonical 冲突、软 404、重定向和抓取失败。
- 分类页与详情页的展示、点击、CTR 和平均位置。
- 孤立页面、重复 title / description、失效来源。
- Core Web Vitals。

### 19.3 GEO 指标

- AI 搜索 referral 和落地页面。
- 按 locale 维护固定问题集和引用采样结果。
- 固定问题集中的引用率、实体识别准确率和事实正确率。
- AI crawler 的抓取覆盖和错误率。
- 价格、功能和状态事实的过期比例。

### 19.4 商业指标

- 申请量、审核通过率和平均审核时间。
- 预审到付款转化率。
- Paid submission 转化率；固定期限模式启用后统计续费率。
- 合作链接健康率和恢复时间。
- Paid submission、Link exchange submission 和标准收录出站链接的点击表现。
- Sponsor 广告申请量、预审通过率、付款转化率、有效广告数、展示与点击表现。
- 管理员免费广告与用户付费广告分开统计，财务指标只计算已付款 `purpose=ad` 订单。

## 20. 测试策略

### 20.1 后端

- URL 规范化与重复实体测试。
- 申请状态机测试。
- 广告申请状态机、独立 session audience 与目录申请隔离测试。
- Placement 状态机与到期测试。
- `paid` / `exchange` active 资格互斥、切换事务与公开申请类型允许列表测试。
- Stripe 验签、幂等、乱序和重复事件测试。
- Stripe Checkout、订单快照和 webhook 只接受 USD，美分换算与非 USD 拒绝测试。
- 人工付款唯一流水测试。
- 反链 HTML 匹配、跳转和错误分类测试。
- SSRF：私网、IPv6、DNS rebinding、重定向到私网和云元数据测试。
- 发布事务和 revalidation job 测试。
- transactional outbox 原子性、Worker 重试、抢占超时和幂等完成测试。
- draft revision 更新期间公开 API 始终返回原 published revision 的隔离测试。
- 来源修正只影响新 revision，当前 published revision 保持原来源集合的隔离测试。
- `site_categories` 是唯一分类来源、placement 不含分类字段的 Schema 与服务层测试。
- 多条 active placement 的 CTA、rel、披露和稳定决胜优先级测试。
- 广告四语言 localization 完整性测试；当前 locale 缺失时不回退其他语言。
- 广告目标 URL 协议、重定向与危险地址检查，以及 ImgAnt `6url` asset 引用检查。
- 广告排序、启停、起止时间、自动过期和稳定决胜测试。
- 广告 preview / commit、ETag / `If-Match` 冲突与审计日志测试。
- 广告 commit 与首页 revalidation outbox 同事务提交测试。
- Paid / Exchange submission 仅创建 placement，永远不自动创建 ad 的边界测试。
- 广告申请只创建 ad 与广告订单，永远不创建 listing placement 的边界测试。
- 用户广告必须关联已审核 `ad_application` 和已付款 `purpose=ad` 订单的约束测试。
- 管理员免费创建广告仅写广告与审计数据，保持 ad application、order 和 payment event 为空的边界测试。
- 管理员免费直录仅创建 editorial placement，保持 paid order 与 payment event 为空的边界测试。
- ImgAnt asset 登记的 host、相册名、image ID、storage key 和重复 hash 测试。
- ImgAnt asset 引用删除阻断、远端失败、unhealthy 回退和发布阻断测试。
- 管理 token、session、scope、CSRF、撤销、到期、CORS、CSP 和 ETag 冲突测试。
- 管理 session / application session 严格限流与敏感 body 日志排除测试。
- 反链 URL registrable domain、跨域跳转人工审核和每跳 SSRF 复核测试。
- 发布事务创建邮件 / Slack outbox、幂等重试和失败告警测试。
- UnoSend / Sendflare 请求路径、鉴权、payload、非 2xx 响应和超时测试。
- 四种 locale 邮件模板、HTML escape、纯文本 fallback、状态链接关闭点击重写测试。
- notification payload 静态加密、发送后清除、日志脱敏和未配置 provider 保留 pending 测试。

### 20.2 前端

- 首页、分类和详情页服务端 HTML 包含主体内容。
- metadata、canonical、robots 和 JSON-LD 快照测试。
- Sponsor 广告从编辑 `ItemList`、相关推荐和网站实体 JSON-LD 中排除，公开 API 使用独立 `ads` 字段的测试。
- 四种 locale 的路由、语言检测、hreflang 双向完整性和 x-default 测试。
- 缺少翻译时不生成 URL、不进入 sitemap、不出现在语言切换器的测试。
- sitemap 只包含合格 URL。
- draft、suspended、archived、removed 状态行为测试。
- 旧 slug 301 和永久删除 410 测试。
- 管理 API 发布预检、operation token 和提交测试。
- 目录状态链接 fragment 清除、HttpOnly application session 与 Referer 无 token 测试。
- 四语言 `/advertise`、广告申请状态页、广告状态 token fragment 清除和独立 HttpOnly session 测试。
- 简单后台的列表、筛选、详情、快捷操作和 scope 控制测试。
- Sponsor 广告卡片固定显示可见广告标识并输出 `rel="sponsored noopener"` 的 HTML 快照测试。
- Sponsor 广告只在具备完整当前 locale 文案时进入对应语言首页的测试。

### 20.3 端到端

- 合作申请 → 审核 → 验证 → 发布 → 连续失败 → 暂停 → 恢复。
- Paid submission → 预审 → Checkout → webhook → 标准收录；固定期限模式继续覆盖到期。
- Sponsor 广告申请 → 预审 → Checkout → webhook → 广告草稿 → 四语言审核 → 首页发布 → 到期或退款下线。
- Stripe 同步卡支付、异步成功、异步失败、Session 过期、事件乱序和全额退款。
- 重复申请合并到现有实体。
- 支付成功后刷新失败，队列重试并最终更新页面。
- 数据库或 Redis 短时故障后的恢复。
- ImgAnt 上传失败时保持草稿，重试成功后登记 asset 并发布。
- 后台与本地 AI 同时修改资源时返回 version conflict，重新读取后安全提交。
- 收录发布成功后申请人收到包含正确 locale 页面链接的邮件，管理员收到 Slack 通知。
- 管理员免费直录网站 → 四语言内容审核 → 发布 → 首页和详情页显示，全程不产生订单或支付记录。
- 管理员创建 Sponsor 广告 → 首页显示 → 调整排序 → 暂停 → 恢复 → 到期隐藏。
- 现有 4 个 Sponsor 广告迁移后，名称、目标 URL、图片、locale 文案、顺序和披露与生产配置一致。

## 21. 实施阶段

| 阶段 | 内容 | 预计工时 |
|---|---|---:|
| P0 | 业务规则、链接政策、价格、退款和内容标准定稿 | 2～3 天 |
| P1 | Go 骨架、Ent Schema、revision / publication、MySQL outbox、Redis、scoped token 鉴权 | 4～6 天 |
| P2 | 数据导入、4 个现有广告迁移、公开 API、ImgAnt 图片迁移 | 3～5 天 |
| P3 | Next.js 四语言路由、ISR、metadata、hreflang、JSON-LD、sitemap | 6～9 天 |
| P4 | 收录与广告申请表单、application session、四语言邮件、Slack 通知、广告 API、管理 API、简单后台（含广告管理）、OpenAPI 与本地 CLI | 7～10 天 |
| P5 | 合作链接 Checker、域名所有权、调度、通知和恢复流程 | 3～5 天 |
| P6 | 收录与广告的 Stripe 异步事件、人工付款、退款、订单和到期流程 | 4～7 天 |
| P7 | 安全测试、迁移演练、SEO QA、灰度上线 | 4～6 天 |
|  | **合计** | **33～51 个开发日** |

工时按一名熟悉 4aicode 代码的开发者估算。设计稿、定价讨论、批量编辑存量内容和外部支付配置耗时单独计算。

## 22. 上线策略

1. 部署后端和数据库，暂时保持现有 JSON 前台。
2. 导入存量数据并完成重复与商业关系审计。
3. 新前台在 staging 对比首页输出、链接和性能。
4. 先上线公开目录 API 与首页数据切换。
5. 分批上线中文分类页和中文详情页。
6. 中文页面稳定后，按英文、日文、韩文顺序完成独立 QA 并发布。
7. 每次 locale 发布都校验 hreflang、sitemap、语言内容和内部链接。
8. 上线提交入口、简单管理后台、管理 API、OpenAPI 和本地 AI CLI。
9. 开启合作链接检查。
10. Stripe 测试模式完成端到端后切换正式支付。
11. 观察两个抓取周期，再扩大各语言详情页发布批次。

## 23. 验收清单

### 23.1 产品闭环

- [ ] 公开目录收录入口只提供 Paid submission 和 Link exchange submission；Sponsor 广告使用独立 `/advertise` 入口和状态机。
- [ ] 两种收录申请均可提交、查询状态和收到通知。
- [ ] 用户可以提交 Sponsor 广告申请，审核后使用 USD 付款，并在广告上线后收到邮件。
- [ ] 收录正式发布后申请人收到上线邮件，管理员收到 Slack 通知。
- [ ] 本地 AI 可以通过 scoped token 完成预审、内容编辑、发布和拒绝。
- [ ] 简单后台可以查看核心数据并完成约定的快捷操作。
- [ ] `/admin/ads` 可以审核用户广告申请，并查看和管理现有 4 个 Sponsor 广告的文案、素材、顺序、状态和有效期。
- [ ] 后台和本地 AI 共用 `/admin/v1`，状态结果与审计一致。
- [ ] Sponsor 广告与 Paid submission 使用独立模型、接口和流程。
- [ ] 管理员免费创建广告不产生 submission、order 或 payment event。
- [ ] Paid submission 只授予标准收录资格，不自动创建广告。
- [ ] 用户广告订单只激活广告展示，不创建或修改网站收录 placement。
- [ ] 广告在四种语言首页按完整 locale 文案、排序、状态和有效期正确展示，并带有可见广告标识与 `rel="sponsored noopener"`。
- [ ] Stripe 和人工付款只激活一份权益。
- [ ] Stripe 异步支付、Session 过期和全额退款状态闭环通过测试。
- [ ] 合作链接检查具备宽限、暂停和恢复闭环。
- [ ] 固定期限到期只结束收录资格，内容页面按编辑状态处理；永久收录没有到期任务。

### 23.2 SEO

- [ ] 首页、分类和详情主体存在于服务端 HTML。
- [ ] canonical、metadata、robots 和 sitemap 与状态一致。
- [ ] draft 与私有页面不会进入索引。
- [ ] `zh-CN`、`en`、`ja`、`ko` 路由返回对应语言完整 HTML。
- [ ] 每个 locale 独立发布，缺少翻译时不回退其他语言正文。
- [ ] hreflang、x-default、canonical 和 sitemap alternates 一致且双向完整。
- [ ] slug 变更生成 301，永久删除使用 410。
- [ ] 商业关系与 rel 属性一致。
- [ ] 多条有效关系按固定优先级生成 CTA、rel 和完整披露。
- [ ] 存量薄详情页分批发布。
- [ ] Core Web Vitals 达到目标。

### 23.3 GEO

- [ ] 实体拥有稳定 ID、规范 URL、来源和更新时间。
- [ ] 四种语言页面共享实体 ID，并保留各自内容版本和审核记录。
- [ ] 详情页提供直接回答式摘要与结构化事实。
- [ ] 公开 API、feed 和 sitemap 可读取。
- [ ] `llms.txt` 明确指向高质量页面并标记实验属性。
- [ ] 引用抽样流程和事实过期指标已建立。

### 23.4 安全与一致性

- [ ] 反链 Checker 通过完整 SSRF 测试。
- [ ] Stripe webhook 验签且幂等。
- [ ] 订单、placement 和发布刷新具有事务或可恢复队列。
- [ ] locale 草稿更新不会改变当前公开 revision，发布切换与 revalidation outbox 同事务提交。
- [ ] 管理写操作有鉴权和审计。
- [ ] 原始 token、支付密钥和响应正文不会进入日志。
- [ ] 邮件状态 token 位于 fragment，并在交换后清除；申请状态 API 只接受短期 HttpOnly session。
- [ ] 反链域名与 canonical registrable domain 一致，替代域名具有人工审核记录。
- [ ] Logo 进入 ImgAnt `6url` 相册并经过安全重编码。
- [ ] 6URL 数据库保存 ImgAnt 公共资源标识，并拒绝错误相册和非允许 host。
- [ ] 管理 token 只保存 hash，scope、撤销、轮换和审计有效。
- [ ] 后台 session 使用 HttpOnly cookie，并通过限流、CSRF、Origin、精确 CORS、CSP、到期和撤销测试。
- [ ] 后台与 AI 并发更新通过 ETag / `If-Match` 防止覆盖。
- [ ] 图片删除具有全量引用检查、失败状态和公开页回退。
- [ ] 高影响操作通过 preview、operation token、version 和幂等键保护。

## 24. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 合作链接形成排名信号交换 | 域名搜索风险 | 自然品牌链接、接受 nofollow、6URL 出站 nofollow、透明披露 |
| 付费链接缺少资格标记 | 搜索政策风险 | 强制 `sponsored`，发布预检阻断不一致 |
| 批量生成薄详情页 | 抓取预算与站点质量下降 | `noindex` 草稿、编辑门槛、分批发布 |
| 商业到期删除内容 | URL 波动和权重损失 | site 与 placement 分离 |
| 反链检查误判 | 错误暂停合作方 | 多次失败、72 小时、错误分类、人工豁免 |
| 用户 URL 触发 SSRF | 内网和云环境风险 | 解析、dialer、重定向、端口和响应限制 |
| webhook 重复或乱序 | 重复权益与订单错误 | event 幂等、条件更新、状态机测试 |
| Stripe 异步结果或退款处理不完整 | 未付款激活、权益未结束或账实不符 | 覆盖异步成功 / 失败、过期、全额退款并校验订单金额归属 |
| ISR 刷新失败 | 页面状态滞后 | 持久化任务、重试、周期 ISR 兜底 |
| 发布提交成功但 outbox 丢失 | 数据库与静态页面长期不一致 | publication 指针与 revalidation job 同一 MySQL 事务写入 |
| 编辑草稿覆盖已发布正文 | 未审核内容提前公开 | 不可变 revision 与 draft / published 双指针隔离 |
| 编辑来源立即改变已发布页面 | 未审核引用提前公开或引用丢失 | 不可变来源记录与 revision-source 固定关联 |
| 发布成功后邮件或 Slack 暂时不可用 | 申请人未及时获知上线、管理员遗漏事件 | 发布事务写 notification outbox，Worker 幂等重试并在后台展示积压 |
| 异步确认邮件需要使用原始状态 token | token 以明文进入数据库、备份或日志 | 仅在内存和加密 outbox payload 中短暂保存，投递后清除且日志脱敏 |
| 存量联盟链接未披露 | 信任与搜索风险 | 迁移审计、关系字段、统一 rel |
| `llms.txt` 效果被高估 | 投入偏离 | 作为实验项，核心投入放在实体、内容和来源 |
| 同一 URL 按浏览器语言返回不同内容 | canonical 与索引混乱 | locale 固定 URL，语言选择使用普通链接 |
| 未审核翻译批量发布 | 多语言薄内容与事实错误 | 每 locale 独立审核、发布门槛和 source revision |
| hreflang 单向或指向未发布页面 | 搜索引擎忽略语言映射 | head 与 sitemap 共用 alternate 服务并做 CI 校验 |
| source locale 更新后派生语言过期 | GEO 引用不一致 | `source_changed` 状态、revision 依赖、翻译差异队列和过期 SLA |
| ImgAnt 上传暂时不可用 | 内容发布缺少图片 | 保持草稿、幂等重试、已有 asset 继续使用 |
| ImgAnt 相册同名或权限变化 | 图片上传到错误位置或失败 | 上线预检相册唯一性，固定 `album=6url`，独立 API Key |
| 状态 token 出现在路径和日志 | 申请信息或支付入口泄漏 | URL fragment 通过单次请求换取短期 session，敏感 body 日志排除与严格限流 |
| 任意外部域名提供反链即可通过 | 无所有权网站冒用合作资格 | registrable domain 一致性和替代域名人工审核 |
| 多条 active placement 产生不一致 rel | 付费关系错误输出普通链接 | 服务层固定优先级并输出解析后的 CTA、rel 与披露 |
| placement 同时承担编辑分类 | 付款改变实体分类与目录语义 | `site_categories` 管理编辑分类，placement 仅控制收录资格 |
| Sponsor 广告与 Paid submission 混用 | 用户付款后获得错误权益，业务披露和数据状态失真 | `ads` 与 `placements` 独立建模，订单使用 purpose 与 XOR 约束，服务层分别激活广告或收录资格 |
| Sponsor 广告缺少明确标识 | 用户信任和搜索政策风险 | 服务端固定输出可见“Sponsor / 广告”标识与 `sponsored noopener`，发布预检强制校验 |
| 广告目标 URL 或图片输入不安全 | SSRF、恶意跳转、钓鱼或破图风险 | URL 协议与地址检查、重定向复核、ImgAnt asset 登记与引用健康检查 |
| 当前源码缺少线上 4 个广告配置 | 迁移遗漏、链接或排序失真 | 从实际生产配置或导出取得准确数据，并逐语言执行迁移前后快照对比 |
| 后台与 AI 并发编辑 | 草稿或发布指针被静默覆盖 | ETag / `If-Match` 和 409 冲突处理 |
| ImgAnt 外部删除或登记伪造 | 破图和资产信任错误 | scoped token、结构验证、引用检查、健康状态与占位回退 |
| 简单后台范围持续膨胀 | 延迟核心 API 与 SEO 页面 | 固定首版页面与快捷操作，复杂工作流交给本地 AI |

## 25. 明确排除

首版排除以下能力：

- 自动生成并直接发布大量 AI 文案。
- 售卖 dofollow 链接或关键词锚文本。
- 自动创建 Stripe Product / Price。
- 申请方完整账号、团队和多租户系统。
- 无人工审核的自动翻译发布。
- 复杂竞价排名。
- 自动抓取全网网站并批量建页。
- 专用搜索引擎集群。
- 完整 CMS、拖拽页面编辑器和复杂 BI 报表。
- 多档付费套餐、套餐升级和加购。
- 广告竞价、实时库存销售和无人审核自动上刊。
- 分类置顶、付费排序和 Sponsor 模块之外的推荐位商品。

## 26. 评审问题

实施前需要确认：

1. 单一 Paid submission 的 USD 价格，以及采用一次性永久收录还是固定有效期。
2. Link exchange submission 是否接受申请方使用 `nofollow`。
3. 付费和合作到期后，合格网站详情页的长期保留政策。
4. Stripe 支持的支付方式，以及人工支付宝 / 微信的对账流程。
5. 退款窗口、拒绝退款条件和违规下架规则。
6. 四种语言的分类范围、翻译责任人和内容发布最低标准。
7. source locale 事实变更后，其他派生语言允许保持旧版本的最长时间。
8. 本地 AI token 的操作者、scope、轮换周期和目标审核 SLA。
9. 现有重定向 / 联盟链接的业务归属。
10. 付费与合作 placement 覆盖全部四种语言，还是按 locale 套餐销售。
11. 线上 4 个 Sponsor 广告的实际生产配置位于何处，以及采用配置读取还是数据导出完成迁移。
12. 用户 Sponsor 广告的 USD 价格、展示期限、locale 覆盖、可售位置、库存上限和退款规则。

## 27. 自审记录

本文档完成四轮自审，覆盖产品闭环、SEO / GEO、数据一致性、安全性和实施可行性。

| 级别 | 发现 | 修正结果 |
|---|---|---|
| 严重 | 商业状态直接控制详情页会导致付款到期或反链异常时 URL 消失 | 引入 `sites` 与 `placements` 分离；商业状态只控制权益 |
| 严重 | 强制 dofollow 互链会放大链接交换风险 | 合作计划接受自然品牌链接与 nofollow；6URL 出站默认 nofollow |
| 严重 | 用户提供的 backlink URL 可被利用访问内网 | 增加完整 SSRF 防护、重定向复核和测试清单 |
| 高 | 付款发生在审核前会增加退款和垃圾站点争议 | 流程调整为编辑预审后付款 |
| 高 | Stripe 回跳页可能被误用为付款真相 | 订单只由签名 webhook 或管理 API 人工确认驱动 |
| 高 | ISR 刷新属于跨系统非原子操作 | publication 状态与 `revalidation_jobs` 在同一事务写入，Worker 提交后消费并重试 |
| 高 | JSON 存量直接生成详情页会制造薄内容 | 导入页保持 noindex，达到编辑门槛后分批进入 sitemap |
| 高 | 当前资源包含重定向和疑似联盟链接 | 迁移加入 canonical target、outbound URL 与关系审计 |
| 中 | 反链单次超时会误暂停正常合作方 | 使用连续 3 次、72 小时、补查和人工豁免 |
| 中 | 套餐更新可能重写历史权益 | order 保存金额、期限和位置快照 |
| 中 | 首版多档套餐会增加定价、库存和后台配置复杂度 | 收敛为一个 active plan，订单继续保存权益快照 |
| 中 | Paid submission 与 Sponsor 广告共用权益会扩大产品和排序复杂度 | Paid submission 与 `ads` 拆成独立模型；公开目录收录申请类型固定为 paid 与 exchange，广告申请进入独立状态机 |
| 中 | 收录发布与线上通知跨系统执行会出现部分失败 | 发布事务创建邮件 / Slack outbox，提交后重试且不回滚已发布内容 |
| 高 | 直接照搬同步邮件调用会让未配置 provider 被当作成功且缺少重试 | 复用 4aicode Sender，并强制所有生产邮件经 durable outbox；未配置时保留 pending |
| 高 | 异步发送状态链接需要暂存原始 token | notification payload 加密存储、日志脱敏、发送或到期后清除 |
| 中 | `llms.txt` 缺少稳定效果标准 | 降级为实验项，GEO 重点放在实体、来源和可引用内容 |
| 中 | 付费到期页面处理定义不清 | 结束 paid placement，详情页由编辑状态决定 |
| 中 | 任意远程 Logo 会扩大安全和性能问题 | ImgAnt `6url` 相册、重编码、hash 去重和 host 允许列表 |
| 高 | 多语言使用运行时正文回退会让 locale URL 与页面语言不一致 | 各 locale 独立发布；缺失翻译时不生成公开页面 |
| 高 | 四套文案容易出现事实漂移 | 引入共享实体事实、source revision、`source_changed` 和翻译状态 API；任一语言均可作为 source locale |
| 中 | 独立实现 head hreflang 与 sitemap alternates 容易产生漂移 | 两处复用同一 alternate 生成服务并加入双向校验 |
| 高 | 本地 AI 使用长期全权限 token 会扩大误操作和泄漏影响 | scoped token、内容与支付权限分离、hash 存储、撤销和轮换 |
| 高 | AI 重试写请求可能造成重复发布、付款或退款 | `Idempotency-Key`、资源 version 和两阶段高影响操作 |
| 中 | 仅靠 AI API 会降低日常数据观察效率 | 增加简单后台，保留 API 作为唯一业务入口 |
| 中 | 6URL 服务端再保存 ImgAnt API Key 会扩大秘密管理范围 | 首版本地 AI 直接上传，6URL 只登记公开资源标识 |
| 中 | placement 全局激活会让未包含的语言获得商业展示 | 增加 `placement_locales`，查询时与已发布 locale 内容取交集 |
| 中 | 移除管理员账号后 Schema 仍引用 user ID | 编辑与审核字段统一引用 `admin_api_tokens.id` |
| 中 | 管理 API 缺少版本前缀会增加 AI 客户端升级风险 | 管理接口统一使用 `/admin/v1`，OpenAPI 与 CLI 绑定明确版本 |
| 严重 | 草稿和已发布内容共用可变 locale 行会让未审核编辑提前公开 | 改为不可变 content revision 与 draft / published publication 双指针 |
| 高 | site 级可变来源会绕过 locale revision 发布隔离 | 来源改为不可变记录，并由 content revision 固定引用集合 |
| 高 | 状态 token 位于 URL path 会进入历史、日志、分析和 Referer | 邮件链接改用 fragment，通过单次请求换取短期 HttpOnly application session 后清除 token |
| 高 | 先提交内容再创建刷新任务会在进程故障时丢失刷新 | 改为 transactional outbox，publication 指针和 revalidation job 同一 MySQL 事务提交 |
| 高 | 把申请方式或商业资格写入分类会让付费影响目录语义 | `site_categories` 作为唯一编辑分类来源，placement 不保存分类字段 |
| 高 | 多条有效 placement 的 CTA、rel 和披露结果缺少确定性 | 定义 paid / affiliate、exchange、editorial 优先级与稳定决胜规则 |
| 高 | 任意页面提供 6URL 链接都可能冒用合作资格 | backlink registrable domain 必须匹配 canonical domain，替代域名人工审核 |
| 高 | Stripe 只处理同步完成事件会漏掉异步支付、过期和退款 | 补齐异步成功 / 失败、Session 过期和全额退款，并校验订单金额与 livemode 归属 |
| 高 | 管理 session 交换和跨域策略缺少显式防护 | 增加独立限流、敏感 body 日志排除、精确 CORS、严格 CSP 和 session 轮换 |
| 中 | 后台与本地 AI 并发保存会静默覆盖修改 | 两端统一使用 ETag / `If-Match`，版本冲突返回 409 |
| 中 | ImgAnt 登记属于受信任输入且外部删除可能产生破图 | 明确信任边界、全量引用检查、资产健康状态、失败重试与占位回退 |
| 高 | 管理员免费直录复用 paid order 会伪造收入与商业关系 | 免费直录创建 `editorial` placement，并通过约束保持 submission、order 和 payment event 为空 |
| 高 | 管理员免费配置 Sponsor 广告触发支付流程会污染订单与广告状态 | `origin=admin` 广告通过管理服务直接写入，数据库约束其 ad application、order 和 payment event 为空 |
| 高 | 当前源码无法提供线上 4 个广告的准确数据 | 迁移前读取实际生产配置或导出，逐项校验并保存首页对比结果 |
| 中 | 原 24～34 天估算未覆盖 revision、application session、广告申请、简单后台和支付边界 | 调整为 33～51 个开发日，内容生产与外部配置继续单独估算 |

### 27.1 自审结论

第四轮自审已关闭已发布内容隔离、状态 token 泄漏、outbox 原子性、分类耦合、多 placement 决策、反链所有权、Stripe 异步状态、后台并发安全和 Sponsor 广告边界等结构性缺口。首版语言确定为中文、英文、日文和韩文，多语言采用固定 URL、独立审核、不可变 revision 与共享实体事实。管理方式采用简单后台与本地 AI 共用 `/admin/v1`。Sponsor 就是 Ad，使用独立 `ads` 与 `ad_applications` 模型；用户可申请并付款，管理员可免费配置。Paid submission 只获得标准收录资格。当前保留十二项业务决策，其中收录与广告价格、合作链接政策、到期保留、退款规则、翻译更新 SLA、套餐语言范围和现有广告数据来源会直接影响 Schema 约束、页面披露、迁移与状态机；这些决策确认后进入接口细化和实施拆分。

## 28. 参考政策与实现依据

- [Google Search Central: Spam policies](https://developers.google.com/search/docs/essentials/spam-policies)，重点关注 link spam、付费链接和过度链接交换。
- [Google Search Central: Qualify outbound links](https://developers.google.com/search/docs/crawling-indexing/qualify-outbound-links)，付费关系使用 `rel="sponsored"`。
- [Google Search Central: Structured data general guidelines](https://developers.google.com/search/docs/appearance/structured-data/sd-policies)。
- Google Search Central：Sitemaps、canonical 与 JavaScript SEO 基础指南。
- 4aicode：Go / Gin / Ent / MySQL / Redis、API key 鉴权、Stripe 幂等 webhook、后台 Worker 和 systemd + nginx 部署模式；邮件复用 `backend/internal/mailer`，通知 outbox 复用 `backend/internal/notifications` 与 `backend/ent/schema/notification_event.go`。
- [ImgAnt API 文档](https://imgant.com/api-docs)：`POST /api/v1/api-upload`、`X-API-Key`、multipart `file` + `album`、`api-albums` 与 CLI `--album` 工作流。

实施前再次核对搜索平台和支付平台的最新官方政策，并把变化写入 `/listing-policy` 与测试清单。

## 29. 实施进度

### 29.1 已完成基础

- 建立 `backend/` Go 服务、Ent Schema、MySQL 连接、显式迁移命令和四语言种子数据；API 启动过程不执行 DDL。
- 建立 category / site 不可变 revision 与 locale publication、placement、Sponsor Ad、两类申请、订单、后台 token、审计和 outbox 数据表。
- 实现 `/healthz`、`/api/v1/locales`、`/api/v1/catalog`；catalog 只读取已发布 locale revision，并将 Sponsor 广告放入独立 `ads` 字段。
- 实现 scoped administrator token、一次性显示 token 的本地 CLI、核心后台只读 API 和管理员免费直录网站 API。
- 管理员免费直录通过事务创建 draft site、locale revision、editorial placement 与审计记录，不创建 submission 或 payment order。
- 实现当前 JSON catalog 的幂等导入 CLI；导入内容保持中文草稿，现有 JSON 继续作为前台回退。
- Next.js 首页支持 `SIXURL_CATALOG_SOURCE` 数据源开关、ISR 和 Sponsor 广告服务端渲染。
- 组件移出 `pages/`，清除意外公开的 `/components/*` 路由。
- 静态 JSON 移至 `data/`，清除由配置文件目录产生的无意义 `/api` 路由。
- 建立当前管理接口的 OpenAPI 3.1 文档与前后端 CI。
- 修复首轮代码审查发现的 MySQL 长 URL 唯一索引、revision 归属校验、商业实体 XOR 约束、后台逐路由 scope、运行时 JSON 快照回退和 registrable domain 规范化问题。
- Next.js 升级到 15.5 安全维护版本，生产依赖审计清零。

### 29.2 当前验证

- Go：`go test ./...`、`go vet ./...`、两个 CLI / API 二进制构建通过。
- Next.js：`yarn lint` 与 `yarn build` 通过。
- SQLite 集成测试覆盖 published revision 隔离、广告 locale 不回退、商业 rel 解析，以及管理员免费直录不产生订单。
- MySQL 实例、生产邮件、Slack、Stripe 和 ImgAnt 使用项目所有者提供的环境配置后执行联调。

### 29.3 已完成申请与支付后端

- 实现 Paid submission、Link exchange submission 与 Sponsor Ad application 公开 API；Turnstile、IP 限流、URL / locale / 反链域名校验和泛化 `202` 响应已经接入。
- 实现两套独立状态 token audience、HMAC 数据库存储、fragment 状态链接、30 分钟 HttpOnly session、Origin + CSRF 写保护和申请人补充材料 API。
- 实现后台预审、要求补充与拒绝 API；条件状态转换、管理员 token、审计日志、单一 active USD plan / offer 和订单价格权益快照处于同一事务。
- 实现 UnoSend / Sendflare 邮件、Slack 管理员通知与 durable notification Worker；outbox payload 使用 AES-256-GCM，支持 claim、指数退避、stale 重入队、过期清理和成功后密文清除。
- 实现 Stripe USD Checkout 与签名 webhook；支付完成由 webhook 推动订单及申请进入 `paid`，回跳参数不改变付款状态。
- 实现 ImgAnt `6url` 相册资产登记和管理员免费 Sponsor Ad 草稿 API；管理员广告不创建 application 或 payment order。
- 实现 listing / sponsor 商业套餐迁移种子，金额、期限、locale 与 Stripe Price ID 由环境变量写入 active offer。
- 实现公开 API OpenAPI 3.1 文档并扩展管理 API 文档；新增流程由 SQLite 集成测试覆盖 token 不回传、状态 session、预审订单和通知副作用。

### 29.4 下一批实现

- 通用内容 preview / commit、ETag 更新与分类编辑发布流程。
- 四语言公开路由、详情页、hreflang、sitemap、JSON-LD、GEO 来源和索引发布门槛。
- 现有 4 个 Sponsor 广告迁移和生产 Stripe / Email / Slack / ImgAnt 联调。
- 简单管理后台页面；后端 API 继续作为后台与本地 AI 的统一业务入口。

### 29.5 发布与反链闭环进度

- 完成目录申请发布 API：四语言 approved revision、publication 指针、site、category、paid / exchange placement、订单 site 绑定、管理员审计、申请人通知与 revalidation job 在同一事务提交。
- Paid submission 仅允许已付款订单发布；Exchange submission 在事务前执行 SSRF-safe 首次反链检查并将通过结果写入历史。
- 反链检查器禁用环境代理，拒绝私网、回环、链路本地、CGNAT、文档与保留地址；每次重定向重新解析并限制为 5 次，HTML 响应限制为 2 MiB。
- 自然品牌链接与 `nofollow` 均视为有效；连续三次 missing / blocked 或一次 unsafe 会暂停 exchange placement，通过复查后自动恢复因反链失败产生的暂停。
- 完成申请人限流 recheck、管理员 placement recheck 与每日到期扫描 Worker；所有检查保存状态、HTTP 信息、匹配链接、rel、重定向链和错误码。
- 完成支付审查修复：支持 Stripe 异步成功、Session 过期与异步失败后的 Checkout 重试，并强制 PaymentEvent 完成标记参与事务成功条件。
- 完成共享 Stripe 账号隔离：Checkout、PaymentIntent 和 Refund 写入 6URL 专属 metadata；Webhook 以 metadata、本地 Session 和 PaymentIntent 判断事件归属，兼容旧 Checkout，并忽略其他产品和不支持的事件。
- 完成 revalidation outbox Worker 与 Next.js `/api/internal/revalidate`：请求使用 HMAC-SHA256、5 分钟时间窗、event ID 短期防重放、路径允许规则、条件 claim、stale 恢复与指数退避。

### 29.6 Sponsor 与退款闭环进度

- 完成付费 Sponsor 上刊 API：仅接受 `paid` 申请和唯一已付款广告订单，将健康 ImgAnt `6url` 素材、四语言 localization、订单期限快照、active 广告、申请 `published` 状态、审计、邮件 / Slack 通知及首页 revalidation job 在同一事务提交。
- 上刊邮件包含线上展示页、开始时间和结束时间；广告状态 API 返回订单退款时间及广告展示状态、期限和 version。
- 完成管理员免费广告的 `commit`：支持 active、paused、canceled、排序与展示时间，使用 ETag、`If-Match` 和 version 防止后台与本地 AI 并发覆盖；省略排期字段时保留原值，显式 clear 仅适用于管理员广告，用户付费广告排期由订单权益锁定；commit 同事务刷新四语言首页。
- 完成广告到期 Worker：条件更新到期 active 广告为 `expired`，写审计与首页刷新；广告主收到到期邮件，管理员可通过 Slack outbox 获知状态。
- 完成 Stripe 全额退款 API 和 `refund.updated`：同步成功时撤销 paid placement 或 Sponsor 广告，异步退款使用 `refund_pending`；成功后下架权益、保留网站内容、写通知与刷新任务，失败或取消时恢复订单为 `paid`。
- 完成人工订单 `mark-paid` API；仅允许 `provider=manual,status=pending_manual`，要求 `Idempotency-Key`、外部付款引用和审计说明；相同 key 与相同请求安全重试，key 复用于不同请求时返回冲突。
- SQLite 集成测试覆盖付费广告原子上刊、管理员广告乐观并发 commit、广告到期、同步退款下架和异步退款 webhook 完成。
