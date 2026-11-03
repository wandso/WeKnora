# 水位—库容曲线计算工具

一个单文件网页小工具：选水库 → 看基础信息与曲线表 → 水位 ⇄ 库容 双向查算。

## 打开方式

双击 `index.html` 即可（纯前端、离线可用、无需起服务）。

## 功能

| 功能 | 说明 |
| --- | --- |
| 水库下拉框 | 350 座水库（大中型 20 + 小型 330），支持按名称 / 乡镇 / 规模搜索，按工程规模分组 |
| 基础信息 | 工程等别、集雨面积、死水位/死库容、正常蓄水位/正常库容、校核洪水位/总库容、设计洪水位、防洪(兴利)库容、所在地 |
| 曲线表 | 选中水库后按当前步长自动生成，含库容增量、相对死库容等列，可导出 CSV（Excel 可直接打开） |
| 双向查算 | 输入水位得库容；输入库容得水位。同时给出所在区间、内插方式与偏差 |
| 步长 / 精度 | 步长 0.05～2.0 m；精度低/中/高（0.1 m·1 万m³ / 0.01 m·0.1 万m³ / 0.001 m·0.01 万m³） |
| 精度评估 | 底部实时给出「按步长表内插」相对原始曲线的最大/平均偏差，用来判断步长是否够细 |
| 曲线来源 | 默认用实测曲线；可切换「死/校核两锚点」模式做对比 |
| 取值范围 | 默认死水位 → 校核洪水位；也可切到曲线全段 |
| 界面主题 | 默认跟随宿主/系统深浅色，右上角按钮可手动切换；`?theme=dark` 可强制 |

## 界面

视觉参照 [ReactBits](https://reactbits.dev) 的组件语言重做过一版，全部用原生 CSS/JS 实现，无外部依赖：

| 参考组件 | 落在哪里 |
| --- | --- |
| Soft Aurora | 页面背景三层模糊光斑，缓慢漂移 |
| Dot Field + Grain | 22px 点阵 + SVG 噪点，底部渐隐 |
| GradientText | 标题渐变底色缓慢流动 |
| SpotlightCard | 卡片内跟随光标的径向柔光 |
| StarBorder | 「双向查算」卡片描边巡游的高光点（`@property --beam-a` 驱动 conic-gradient） |
| TiltedCard | 两个查算框的 3D 倾斜 + 反光层 |
| AnimatedTabs | 分段控件的滑块，按按钮实测宽度定位，随窗口重排 |
| CountUp | 结果数字变化时 460ms 缓动滚动 |
| ShinyButton | 导出按钮与普通按钮的扫光 + 磁吸位移 |

细节约定：首次进入水库时曲线有 1s 生长动画，之后改输入只更新标记点不再重绘动画；`prefers-reduced-motion` 下所有动效自动关闭（背景光斑直接隐藏）。

## 数据来源

Navicat 连接 `LH_Water_JDBC`（10.33.120.181:8527），库 `sjy_project_basic`：

- `att_res_lm_base` 大中型水库基础信息（20 条）
- `att_res_s_base` 小型水库基础信息（330 条）
- `fhgc_res_lm_rwacdr` 大中型水库水位、面积、库容、泄量关系（6631 点）
- `fhgc_res_s_rwacdr` 小型水库水位、面积、库容、泄量关系（5751 点）

单位：水位 m，库容 万m³（1 亿m³ = 10 000 万m³）。

> 说明：临海本地库 `lh_water_basic.st_zvarl_b` 也有库容曲线（178 座），但存在多条曲线被复制到不同测站、
> 库容非单调、大量空值等问题（如 70400300 下岸水库 163 m 处库容 2.5487 万m³、164 m 处反而 0.4557 万m³），
> 故未作为主数据源。若需并入，可在 `export_from_db.py` 里追加一张表并统一成 `STCD → 名称` 的映射。

## 算法

1. **清洗**：按水位排序、同水位取均值、库容单调不减（前值压平）、剔除空值。
2. **补齐端点**：若基础信息中的死水位低于实测曲线起点，且死库容不大于起点库容，则把（死水位, 死库容）
   作为首点插入；校核洪水位/总库容同理补在末端——保证曲线端点与基础信息一致，避免负库容。
3. **内插**：单调三次插值（PCHIP / Fritsch–Carlson），光滑且不会出现倒灌；超出实测点按端点斜率线性外推并标注。
4. **反查**：对单调曲线做二分（80 次迭代，收敛到 1e-7 m）。
5. **查表模式**：按步长重采样成表，表内线性内插——即人工查表的方式，偏差由底部「精度评估」量化。
6. **两锚点模式**（用户建议的简化方案）：
   `V = V死 + (V总 − V死) · ((Z − Z死)/(Z校 − Z死))^2`，只依赖死水位/死库容与校核洪水位/总库容。

## 目录

```
reservoir-curve-tool/
├── index.html            # 成品（数据已内嵌，直接打开）
├── template.html         # 页面模板，数据位置为 /*__DATA__*/
├── reservoir_data.json   # 导出的数据快照
├── export_from_db.py     # 从数据库导出 json
├── build.py              # json + 模板 → index.html
└── README.md
```

## 数据更新

```bash
set DB_HOST=10.33.120.181
set DB_PORT=8527
set DB_USER=lh_water_basic
set DB_PASS=******
python export_from_db.py
python build.py
```

## 分享链接

页面支持 `#水库名` 与查询参数，例如：

```
index.html#牛头山水库
index.html?src=anchor&step=0.5&prec=1#长潭水库
index.html?theme=dark#长潭水库
```

参数：`src=curve|anchor`、`range=anchor|all`、`mode=table|raw`、`step=0.1`、`prec=1|2|3`、`theme=light|dark`。

## 集成到 WeKnora 工具箱

工具作为内置页签挂在「工具箱」下，入口：左侧菜单 → 工具箱 → **水库库容**。

| 文件 | 改动 |
| --- | --- |
| `frontend/public/reservoir-curve-tool.html` | 由 `build.py` 同步过来的成品页（iframe 直接加载） |
| `frontend/src/views/toolbox/ReservoirToolPanel.vue` | 面板组件：iframe + 主题跟随 + 新窗口打开 |
| `frontend/src/config/toolbox.ts` | 新增 `reservoir` 页签（skills → minitools → reservoir → mcp → browserconnection） |
| `frontend/src/config/settingsAccess.ts` | `reservoir: 'viewer'`，所有成员可用，无部署能力门禁 |
| `frontend/src/views/toolbox/Toolbox.vue` | 挂载面板、注册角标汇总 |
| `frontend/src/i18n/locales/*.ts` | 6 种语言的 `reservoirTool.*` 文案 |

主题通过 `postMessage({ type: 'weknora-theme', theme })` 下发，宿主机切换深浅色时内嵌页即时重绘，不重新加载 iframe。

更新数据后只需重跑：

```bash
python export_from_db.py   # 拉最新数据
python build.py            # 同时更新 index.html 与 frontend/public/
```
