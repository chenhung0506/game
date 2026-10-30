# game

兩人即時對戰小遊戲：皮卡丘、卡比獸、噴火龍、菊草葉四選二（連線後在選角畫面選，兩人可以選同一隻；美術是用形狀畫的佔位圖）。Go 伺服器 + 單頁 Canvas 前端，
WebSocket 同步。上線網址 https://game.linch.uk/ 。

## 指令

```bash
go vet ./... && go test -race ./...     # 測試（改任何東西都要跑）
PORT=8099 go run .                      # 本機啟動，開 http://localhost:8099/?room=abc
./deploy.sh                             # 部署（見下）
```

本機試玩要開**兩個分頁**（第一個進房的是皮卡丘，第二個是卡比獸）。網址不帶參數是手機直式版，
加 `&landscape` 是網頁橫式版。環境變數：`PORT`（預設 8080）、`CONFIG`（設定檔路徑，預設 `config.json`）。

## 檔案

| 檔案 | 內容 |
|---|---|
| `game.go` | 全部遊戲規則：`Step()` 每個 tick 推進一次，各技能（伏特攻擊、打雷、電球、跳躍落地、死光、急凍拳）都在這裡 |
| `config.go` | `Config` 結構、預設值、驗證。設定檔的欄位名稱和註解以這裡為準 |
| `config.json` | **實際在跑的數值**（會被打包進映像、也掛載到容器）。改這個調手感 |
| `main.go` | 房間、WebSocket、30 Hz 迴圈、`/health`、靜態檔；`hi` 訊息（連線時送給客戶端的設定） |
| `static/index.html` | 整個前端：繪圖、鍵盤、手機搖桿與按鈕、特效 |
| `*_test.go` | 測試。想知道某個行為怎麼規定，先看對應的測試 |
| `deploy.sh` | 部署腳本 |
| `Dockerfile` | 多階段建置，容器內 `/app/game`、`/app/static`、`/app/config.json` |

## 架構重點

- **伺服器說了算**：客戶端只送「按住了哪些鍵」`{dx,dy,skill,razor,vine,punch,beam,ball,thunder,ax,ay}`，
  伺服器每 1/30 秒算一次，把所有狀態廣播給兩人。所以所有時間類參數實際上都以 tick（0.033 秒）為單位。
- **選角**：先進房的是 slot 0、後進的 slot 1，第三個收到 `full`；角色由玩家選，存在 `Player.Kind`（0 皮卡丘、1 卡比獸、2 噴火龍、3 菊草葉），快照每人帶 `k`。連線後伺服器送 `hi`（設定，`radius`/`maxhp` 依角色編號）與 `lobby`（`picks`、`here`）；客戶端送 `{"pick":k}`，兩人都選好才開打（`room.choose`）。不存在的角色、或比賽中選都會被拒絕，有人離開就回到選角畫面（另一人的選擇保留）。技能與判定看 `p.Kind`，不要看 slot；`NewGame` 的預設是 slot 0 皮卡丘、slot 1 卡比獸，所以大部分舊測試照舊。
- **場地依開房者的版本決定**：頁面連線時帶 `mode=mobile|web`，只有第一個進房的人有效，兩種場地大小在
  `config.json` 的 `court`。`length` 是兩個角色之間的方向（網頁橫向、手機直向）。
- **手機直式 = 把場地轉 90 度**：皮卡丘逆時針、卡比獸順時針，所以自己永遠在畫面下方。方向鍵與搖桿
  都以「畫面方向」為準，再轉回世界座標送給伺服器。角色圖、文字、閃電保持直立。
- **走路與衝刺是 8 向，瞄準可以任意角度**：搖桿同時送 `dx,dy`（8 向，走路用）和 `ax,ay`（精確方向，
  急凍拳與地球上投用）；鍵盤沒有 `ax,ay`，瞄準沿最後按住的方向。電球不看搖桿，蓄力時瞄準自動追著敵人。
- **前端不寫死數值**：技能的範圍、時間、冷卻、血量、體型都由 `hi` 訊息從 `config.json` 帶過去。
  新增參數時要同時改 `main.go` 的 `hi` 和前端。
- 房間只存在記憶體裡，上限 1000，房間代號 `^[a-z0-9]{1,16}$`；WebSocket 只接受同源。

## 技能與按鍵

| 角色 | 按鍵 / 手機按鈕 | 技能 |
|---|---|---|
| 皮卡丘 | 空白鍵 / 主按鈕 | 伏特攻擊：全身纏電（特效大小跟著 `radius`，碰到敵人的範圍＝`radius`＋對方身體半徑）朝按住的方向衝 `distance`／`duration` 秒，可連用 `max_uses` 次；第一次起 `use_window` 秒內沒用完，或用完最後一次，就進冷卻（`cooldown`）。撞到敵人（受 min/max_height 限制）受 `damage`、麻痺 `paralyze_duration` 秒（`Player.ParalyzeT`：和藤鞭一樣不能走、不能用位移技能，其他技能照用），一次衝刺只打一下；按鈕上顯示剩餘次數 |
| 皮卡丘 | L / 小按鈕「打雷」 | 敵人頭上出現會跟隨的陰影，之後連續落雷；施放時定身 |
| 皮卡丘 | K / 小按鈕「電球」 | 蓄力（定身）後朝敵人所在方向丟出電球（蓄力時瞄準線追著敵人），碰到敵人、牆、或血量歸零就爆炸並擊退 |
| 卡比獸 | 空白鍵 / 主按鈕 | 跳躍，落地有震波（半徑 `jump.slam_radius`） |
| 卡比獸 | K / 小按鈕「死光」 | 破壞死光：蓄力、扇形掃射、硬直；可反覆擊中並擊退，也會打電球 |
| 卡比獸 | J / 小按鈕「冰拳」 | 急凍拳：蓄力、朝瞄準方向衝刺，打中的敵人被冰凍（不能動、不能用技能） |
| 噴火龍 | 空白鍵 / 主按鈕 | 飛行 `fly.duration` 秒：只是移動變快；打不打得到它看各技能的高度範圍（見下）對上 `fly.height`；冷卻從降落才開始 |
| 噴火龍 | K / 小按鈕「噴火」 | 在 `duration` 秒內噴出 `quantity` 團小火球，每團朝噴出當下敵人位置、再隨機偏 ±`random_offset_angle` 度；可邊走邊噴；火球有 HP，會被死光、落地震波、急凍拳、電球爆炸打掉；打到人就消失，不分高度 |
| 菊草葉 | 空白鍵 / 主按鈕「葉刃」 | 葉刃連斬：朝按住的方向跳 `jump_distance`（空中 `jump_duration` 秒、最高 `jump_height`，落地不傷人），接著甩葉子 `slashes` 下（每下 `slash_interval` 秒；第一下左上→右下、第二下右上→左下，輪流），每下朝敵人前進 `slash_step`（碰到身體就停），每下在揮到一半時打中半徑 `slash_radius` 內的人 `damage`；技能期間頭上的葉子放大 `leaf_scale` 倍（只影響畫面），並真的被甩過去當刀，原本的刀光留作淡淡的殘影；冷卻從起跳算；被冰凍或抓住會中斷 |
| 菊草葉 | K / 小按鈕「飛葉」 | 飛葉快刀：在身邊 `scatter_radius` 內隨機距離灑出 `count` 片樹葉（`Game.Balls` 裡 `K=ballLeaf`，`Wait` 時掛在原地）；各自隨機等 `min_delay`～`max_delay` 秒後朝當時敵人的位置以 `speed` 衝出，碰到就 `damage` 並消失；樹葉有 `leaf_hp`，會被死光、落地震波、急凍拳、電球爆炸打掉 |
| 菊草葉 | J / 小按鈕「藤鞭」 | 藤鞭：先蓄力 `charge_duration` 秒（藤鞭伸出來，地上出現警示扇形，瞄準還跟著搖桿，期間定身），再朝瞄準方向（此時鎖定）同時揮出左右兩條藤鞭，各從 ±`sweep_angle` 度在 `sweep_duration` 秒內掃向 0 度會合；長 `length`、寬 `width`；一次揮擊只打一下，被打中的敵人 `bind_duration` 秒內不能走路、不能用位移技能（衝刺、跳躍、飛行、急凍拳、地球上投、葉刃連斬），其他技能照用（`Player.BoundT`）；揮擊後還要定身 `recharge_duration` 秒（藤鞭縮回）；冷卻從蓄力開始算 |
| 噴火龍 | J / 小按鈕「上投」 | 地球上投：朝瞄準方向衝 `length`，碰到（地面上的）敵人就抓住（`Held`，不能動不能用技能）、飛到 `high` 再摔下，落地受 `damage` 並播落地震波特效；噴火龍中途被冰凍會鬆手 |

技能判定都是「技能範圍 + 對方身體半徑 `radius`」，另外每個攻擊技能都有自己的**高度範圍**
`min_height`～`max_height`（含頭尾，寫在該技能的設定區塊裡；`Reach.hits`）：目標當下的高度 `Z` 在範圍內才打得到，
高於或低於都打不到。檔案裡的預設：地面技能（死光、落地震波、地球上投）是 0～12（`groundZ`），
電球與急凍拳是 0～60（打得到 `fly.height` 以內的飛行，打不到跳到 100 的卡比獸），打雷與火球 0～1000（等於不看高度）。
`DefaultConfig()` 的電球與急凍拳是 0～12，所以預設值的噴火龍飛行時它們打不到。
電球飛行中碰到敵人，先受 `contact_damage`，再受爆炸的 `explosion_damage`（兩者同一個高度範圍）。
**投射物互撞**：電球、火球、飛葉只要主人不同、飛行路徑相碰（半徑相加，不看高度），就互相扣對方的傷害（電球 `contact_damage`、火球 `damage`、飛葉 `damage`）。火球與飛葉血量歸零就當場消失，電球歸零則下一步爆炸（和被技能打掉一樣）。判定沿著整個 tick 的相對位移，快速的飛葉不會穿過火球；掛在菊草葉身邊還沒射出的葉子不參與。實作在 `clashBalls`，每對都用 tick 開始時的血量，結果與陣列順序無關。

## 設定檔 `config.json`

結構：`court.{mobile,web}`、`pikachu.{max_hp,walk_speed,radius,volt_tackle,thunder,electro_ball}`、
`snorlax.{max_hp,walk_speed,radius,ice_punch,jump,hyper_beam}`、`charizard.{max_hp,walk_speed,radius,fly,fire_breath,seismic_toss}`、`chikorita.{max_hp,walk_speed,radius,leaf_slash,razor_leaf,vine_whip}`。每個欄位的意思看 `config.go` 的註解。

- 每個攻擊技能區塊都有 `min_height` / `max_height`（需 0 ≤ min ≤ max），沒寫就用 `DefaultConfig()` 的值。
- 讀檔時**不認識的欄位或不合理的值會讓伺服器拒絕啟動**（刻意的，避免打錯字默默吃不到設定）。
  改完先跑 `go test ./...`，裡面有測試會載入這份 `config.json`。
- 檔案缺欄位就用 `DefaultConfig()` 的值。**預設值與 `config.json` 不同**（例如預設場地是 2000x1000）；
  測試大多用預設值，所以改預設值要小心。
- 改了欄位名稱要同步改 `config_test.go` 裡的 JSON 字串，舊名稱現在會被當成錯誤。
- 沒有熱更新：改完要重新部署（下一節）。

## 部署

```bash
./deploy.sh        # 本機檢查 + 測試 -> 伺服器 git pull -> 重建並重啟 game 容器 -> 檢查 /health
./deploy.sh -s     # 跳過測試
./deploy.sh -n     # 乾跑，只印出會在伺服器做的事
```

- 部署的是 **`origin/main`**，所以要先 commit 並 push；腳本會擋下沒 commit、不在 `main`、或和遠端不一致的情況。
- 伺服器 `linode-server`（`ssh linode-server`，金鑰登入）：程式在 `~/game`，用 `~/reverse-proxy/docker`
  底下的 docker-compose 的 `game` 服務啟動（映像由 `~/game` 建置，容器綁 `172.17.0.1:3017`）。
- **一定要重建容器，不能只 `docker restart`**：compose 把 `~/game/config.json` 以單一檔案掛進容器，
  `git pull` 會用換檔案的方式更新它，restart 的容器仍然看著舊檔案。`deploy.sh` 已經用 `--force-recreate`。
- 部署會讓進行中的對戰斷線一次，重新整理可重連。
- **不要直接在伺服器上改 `~/game/config.json`**，下次 `git pull` 會衝突。
- 反向代理與 TLS 在另一個 repo：`~/linch/reverse-proxy`（分支 `master`）。`docker/conf.d/game.conf` 是
  `game.linch.uk` 的 443 設定（含 WebSocket 升級標頭），憑證是獨立的一張（`/etc/letsencrypt/live/game.linch.uk/`）。
  nginx 只會載入打包進映像的 `nginx.template.conf` 裡列出的 conf，新增網站要重建那個映像。
  sudo 需要密碼，**不要輸入密碼或繞過**，需要時請使用者自己執行。

## 這個 repo 的慣例

- **直接 commit 到 `main`**（使用者對這個 repo 明確同意，覆蓋全域「不要直接 commit main」的規則）；
  不需要 PR。`git add` 要逐檔指定，不要 `git add .` / `-A`。
- commit 訊息結尾附 Co-Authored-By 行。使用者說「commit 並部署」就是 commit → push → `./deploy.sh`。
- `config.json` 常有使用者自己調過、還沒 commit 的值：commit 前先 `git diff config.json` 看一下，
  把「我加的欄位」和「使用者調的數值」分開提醒，不要擅自改使用者的數值。
- 行為改動要附測試；測試用 `-race`。想確認測試真的有效，可以在暫存副本裡故意弄壞邏輯看測試會不會失敗
  （確認弄壞的版本能編譯，否則不算數）。

## 容易踩的坑

- **JSON 標籤不能兩個欄位共用**：`X, Y float64 \`json:"x"\`` 會讓兩個欄位都被默默丟掉。分開寫，
  並在 `TestPlayerSnapshotCarriesBeamState` 的鍵清單加上新欄位。
- **時間以 tick 為單位**：冷卻、間隔這類會被取整成 1/30 秒的倍數；`hit_interval` 已用「溢出補到下一次」
  讓平均速率正確，但單次間隔仍是 tick 的倍數。小於一個 tick 的間隔等於每 tick 一次。
- 距離與角度判定要沿「整個移動路徑」算（`segDist`、`sweepHits`），否則高速物體會穿過目標。
- 火球與電球共用 `Game.Balls`（`Ball.K` 區分，`Owner` 是發射的 slot）；火球的散射用 `Game.rng`，測試要重現請換成固定種子（`duel` 輔助函式）。`SetKinds` 會重設位置，測試要先 `SetKinds` 再設位置。
- 新增技能的清單：`config.go`（欄位、預設、驗證）→ `game.go` → `main.go` 的 `hi` 與冷卻表 →
  `static/index.html`（按鍵、手機按鈕 `PAD`、冷卻圈 `CD`、繪圖）→ 測試 → `config.json`。
- 手機版沒有在真機上完整測過（只用滑鼠模擬）；改 UI 位置時要考慮窄螢幕（約 360px）。

## 衝撞技的 mass（對撞規則）

伏特攻擊、急凍拳、地球上投的衝刺階段各有 `mass`（預設與 config.json：急凍拳 150 > 地球上投 100 > 伏特攻擊 60）。
兩者**同時在衝刺**且碰到時（`runningMass` / `contact` / `land` / `cut`，game.go）：mass 大的照常生效，小的被打斷、沒效果
（急凍拳贏地球上投＝噴火龍被冰凍、不會抓人；地球上投贏伏特攻擊＝皮卡丘被抓、不會麻痺）；mass 相同則兩邊都被打斷。
對手沒在衝刺時不比較，照常命中。順序無關（兩個 slot 互換結果相同，volt_test.go 兩種都測）。

## 大廳、重選角色與死鬥

- 結果出來後（單場，或死鬥整場結束）畫面上有「重選角色」按鈕，送 `{"repick":true}`：**雙方**都回大廳重選
  （`room.repick`；只在結果顯示中有效，所以另一人同時按也不會重複）。結果顯示 `resultHold`（6 秒）後仍會自動開下一場。
- 大廳有「單場對戰 / 死鬥」切換（`{"team":true|false}`，房間層級，切換時雙方重選；對戰中拒絕）。
- 死鬥：雙方各送一個四隻全上場的順序 `{"order":[2,0,3,1]}`（必須是 0..3 的排列），兩邊都送了就開打。
  一隻 HP 歸零就換順序中的下一隻（`room.boutOver` / `nextRound`，間隔 `roundReset` 2 秒），**活著的角色保留剩下的 HP**，
  位置與技能狀態重置；一方四隻都倒下整場結束（兩邊同時倒下最後一隻 → slot 0，和單場同一個任意規則）。
- 對手的後續順序是秘密：快照的 `tm`（`c` 目前第幾隻、`f` 已倒下的角色、`o` 整場結束、`mw` 勝者）只公開已倒下的，
  自己的順序由前端自己記著。測試用 `room.step(now)`，不必開 socket（team_test.go）。

## 放水（讓分）

大廳的拖拉條，每格 10%、最多 90%（`easeStep` / `easeMax`，`{"ease":30}`）。放水的玩家**最大血量與移動速度**都乘 `(100-放水)%`
（`Game.Ease`、`Game.maxHP`；速度在 Step 的走路/飛行/空中操控那一處統一乘，技能的衝刺不受影響）。
放水只在開打前設定，對戰中拒絕；`Reset`、死鬥換隻都沿用；新加入的玩家從 0 開始。血條仍以角色滿血為基準，所以放水的人血條比較短。
