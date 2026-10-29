# game

兩人即時對戰小遊戲：皮卡丘、卡比獸、噴火龍三選二（連線後在選角畫面選，兩人可以選同一隻；美術是用形狀畫的佔位圖）。Go 伺服器 + 單頁 Canvas 前端，
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
| `game.go` | 全部遊戲規則：`Step()` 每個 tick 推進一次，各技能（衝刺、打雷、電球、跳躍落地、死光、急凍拳）都在這裡 |
| `config.go` | `Config` 結構、預設值、驗證。設定檔的欄位名稱和註解以這裡為準 |
| `config.json` | **實際在跑的數值**（會被打包進映像、也掛載到容器）。改這個調手感 |
| `main.go` | 房間、WebSocket、30 Hz 迴圈、`/health`、靜態檔；`hi` 訊息（連線時送給客戶端的設定） |
| `static/index.html` | 整個前端：繪圖、鍵盤、手機搖桿與按鈕、特效 |
| `*_test.go` | 測試。想知道某個行為怎麼規定，先看對應的測試 |
| `deploy.sh` | 部署腳本 |
| `Dockerfile` | 多階段建置，容器內 `/app/game`、`/app/static`、`/app/config.json` |

## 架構重點

- **伺服器說了算**：客戶端只送「按住了哪些鍵」`{dx,dy,skill,atk,punch,beam,ball,thunder,ax,ay}`，
  伺服器每 1/30 秒算一次，把所有狀態廣播給兩人。所以所有時間類參數實際上都以 tick（0.033 秒）為單位。
- **選角**：先進房的是 slot 0、後進的 slot 1，第三個收到 `full`；角色由玩家選，存在 `Player.Kind`（0 皮卡丘、1 卡比獸、2 噴火龍），快照每人帶 `k`。連線後伺服器送 `hi`（設定，`radius`/`maxhp` 依角色編號）與 `lobby`（`picks`、`here`）；客戶端送 `{"pick":k}`，兩人都選好才開打（`room.choose`）。不存在的角色、或比賽中選都會被拒絕，有人離開就回到選角畫面（另一人的選擇保留）。技能與判定看 `p.Kind`，不要看 slot；`NewGame` 的預設是 slot 0 皮卡丘、slot 1 卡比獸，所以大部分舊測試照舊。
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
| 皮卡丘 | 空白鍵 / 主按鈕 | 衝刺（朝按住的方向） |
| 皮卡丘 | L / 小按鈕「打雷」 | 敵人頭上出現會跟隨的陰影，之後連續落雷；施放時定身 |
| 皮卡丘 | K / 小按鈕「電球」 | 蓄力（定身）後朝敵人所在方向丟出電球（蓄力時瞄準線追著敵人），碰到敵人、牆、或血量歸零就爆炸並擊退 |
| 皮卡丘 | J | 地震（沒有手機按鈕） |
| 卡比獸 | 空白鍵 / 主按鈕 | 跳躍，落地有震波（半徑 `jump.slam_radius`） |
| 卡比獸 | K / 小按鈕「死光」 | 破壞死光：蓄力、扇形掃射、硬直；可反覆擊中並擊退，也會打電球 |
| 卡比獸 | J / 小按鈕「冰拳」 | 急凍拳：蓄力、朝瞄準方向衝刺，打中的敵人被冰凍（不能動、不能用技能） |
| 噴火龍 | 空白鍵 / 主按鈕 | 飛行 `fly.duration` 秒：只是移動變快，電球與急凍拳仍打得到（`reachable`），其他地面技能打不到；冷卻從降落才開始 |
| 噴火龍 | K / 小按鈕「噴火」 | 在 `duration` 秒內噴出 `quantity` 團小火球，每團朝噴出當下敵人位置、再隨機偏 ±`random_offset_angle` 度；可邊走邊噴；火球有 HP，會被死光、落地震波、急凍拳、電球爆炸打掉；打到人就消失，不分高度 |
| 噴火龍 | J / 小按鈕「上投」 | 地球上投：朝瞄準方向衝 `length`，碰到（地面上的）敵人就抓住（`Held`，不能動不能用技能）、飛到 `high` 再摔下，落地受 `damage` 並播落地震波特效；噴火龍中途被冰凍會鬆手 |

技能判定都是「技能範圍 + 對方身體半徑 `radius`」；地面技能（地震、死光、落地震波、電球、急凍拳）
打不到跳在空中的人，打雷不分高度。

## 設定檔 `config.json`

結構：`court.{mobile,web}`、`pikachu.{max_hp,walk_speed,radius,dash,thunder,electro_ball}`、
`snorlax.{max_hp,walk_speed,radius,ice_punch,jump,hyper_beam}`、`charizard.{max_hp,walk_speed,radius,fly,fire_breath,seismic_toss}`。每個欄位的意思看 `config.go` 的註解。

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
