# game

兩人即時對戰小遊戲：皮卡丘、卡比獸、噴火龍、菊草葉、六尾、木木梟、鯉魚王七選二（連線後在選角畫面選，兩人可以選同一隻；美術是用形狀畫的佔位圖）。Go 伺服器 + 單頁 Canvas 前端，
WebSocket 同步。上線網址 https://game.linch.uk/ 。

## 指令

```bash
go vet ./... && go test -race ./...     # 測試（改任何東西都要跑）
PORT=8099 go run .                      # 本機啟動，開 http://localhost:8099/
./deploy.sh                             # 部署（見下）
```

本機試玩要開**兩個分頁**（會自動配成一桌，不需要房號；選角色在畫面上選）。網址不帶參數是手機直式版，
加 `?landscape` 是網頁橫式版。環境變數：`PORT`（預設 8080）、`CONFIG`（設定檔路徑，預設 `config.json`）。

## 檔案

| 檔案 | 內容 |
|---|---|
| `magikarp.go` | 鯉魚王（彈跳、累計進化）與它進化成的紅色暴鯉龍（海浪、擺尾；死光用卡比獸的）：`stepMagikarp` / `stepGyarados` / `stepWaves` / `evolve` |
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
- **配對（沒有房號）**：連線網址不帶房間，只帶 `?mode=`；`hub.join` 把新來的人放進「只有一個真人」的房間（`hub.waiting`：等最久的先；有同樣 `mode` 的優先，沒有就不挑），沒有就開一間新的（伺服器自己編號，玩家看不到）。沒帶 `menu=1` 的連線（測試、舊頁面）就是這樣直接配對。頁面一律帶 `&menu=1`：此時若有人單獨在房裡，`hub.seat` 先回 `{t:rooms,rooms:[{id,wait,stale,pick,bot,team}]}` 讓玩家選（`{join:id}` 加入、`{new:true}` 自己開房、`{rooms:true}` 重抓；頁面每 2 秒重抓一次，沒人等了就自動開房）；`stale` = 超過 `staleAfter`（2 個 ping）沒回 pong，多半是斷線的人，排最後並畫成灰色。那個人若正在和電腦對戰，電腦對局會被中斷（見「單機」）。真人離開後房間裡剩一個人就繼續等，沒人了房間就刪掉。斷線偵測：每 3 秒 ping、10 秒沒回 pong 就斷（`pingEvery`/`pongWait`），幽靈占位最多約 10 秒；代價是手機切到背景超過 10 秒會被踢。已知限制：兩間房各剩一個人時，這兩個人不會被合併，要等新來的人才會各補滿一間。
- **選角**：先進房的是 slot 0、後進的 slot 1，房間滿了就不會再有人被放進去（`full` 只在房間數到 `maxRooms` 時才出現）；角色由玩家選，存在 `Player.Kind`（0 皮卡丘、1 卡比獸、2 噴火龍、3 菊草葉、4 六尾、5 木木梟、6 鯉魚王；進化後變成 7 紅色暴鯉龍，是 `Player.Kind` 的值但**不能選**：`kinds`＝7 是能選的數量，`forms`＝8 是所有 kind 的數量，`hi` 的 `radius`/`maxhp` 長度是 `forms`，每一輪 `Reset` 都把暴鯉龍變回鯉魚王），快照每人帶 `k`。連線後伺服器送 `hi`（設定，`radius`/`maxhp` 依角色編號）與 `lobby`（`picks`、`here`）；客戶端送 `{"pick":k}`，兩人都選好才開打（`room.choose`）。不存在的角色、或比賽中選都會被拒絕，有人離開就回到選角畫面（另一人的選擇保留）。技能與判定看 `p.Kind`，不要看 slot；`NewGame` 的預設是 slot 0 皮卡丘、slot 1 卡比獸，所以大部分舊測試照舊。
- **場地依開房者的版本決定**：頁面連線時帶 `mode=mobile|web`，只有第一個進房的人有效，兩種場地大小在
  `config.json` 的 `court`。`length` 是兩個角色之間的方向（網頁橫向、手機直向）。
- **手機直式 = 把場地轉 90 度**：皮卡丘逆時針、卡比獸順時針，所以自己永遠在畫面下方。方向鍵與搖桿
  都以「畫面方向」為準，再轉回世界座標送給伺服器。角色圖、文字、閃電保持直立。
- **走路與衝刺是 8 向，瞄準可以任意角度**：搖桿同時送 `dx,dy`（8 向，走路用）和 `ax,ay`（精確方向，
  急凍拳與地球上投用）；鍵盤沒有 `ax,ay`，瞄準沿最後按住的方向。電球不看搖桿，蓄力時瞄準自動追著敵人。
- **前端不寫死數值**：技能的範圍、時間、冷卻、血量、體型都由 `hi` 訊息從 `config.json` 帶過去。
  新增參數時要同時改 `main.go` 的 `hi` 和前端。
- 房間只存在記憶體裡，上限 1000；WebSocket 只接受同源。**舊的 `?room=xxx` 連結已移除**（帶了也被忽略），沒有辦法私約房間。

## 技能與按鍵

**電腦版鍵盤對每隻角色都一樣**：WASD 移動（沒有方向鍵）、空白鍵＝主技能、J、K＝兩個次要技能：**J 是手機版左邊那顆小按鈕、K 是右邊那顆**（鍵盤上 J 在左邊，下表每隻角色的 J、K 各是哪一招）。手機版的按鈕位置不變。前端把 J、K 同時送成所有同鍵的欄位，伺服器只看符合角色的那個。

| 角色 | 按鍵 / 手機按鈕 | 技能 |
|---|---|---|
| 皮卡丘 | 空白鍵 / 主按鈕 | 伏特攻擊：全身纏電（特效大小跟著 `radius`，碰到敵人的範圍＝`radius`＋對方身體半徑）朝按住的方向衝 `distance`／`duration` 秒，可連用 `max_uses` 次；第一次起 `use_window` 秒內沒用完，或用完最後一次，就進冷卻（`cooldown`）。撞到敵人（受 min/max_height 限制）受 `damage`、麻痺 `paralyze_duration` 秒（`Player.ParalyzeT`：和藤鞭一樣不能走、不能用位移技能，其他技能照用），一次衝刺只打一下；按鈕上顯示剩餘次數 |
| 皮卡丘 | J / 小按鈕「打雷」 | 敵人頭上出現會跟隨的陰影，之後連續落雷；施放時定身 |
| 皮卡丘 | K / 小按鈕「電球」 | 蓄力（定身）後朝敵人所在方向丟出電球（蓄力時瞄準線追著敵人），碰到敵人、牆、或血量歸零就爆炸並擊退 |
| 卡比獸 | 空白鍵 / 主按鈕 | 跳躍，落地有震波（半徑 `jump.slam_radius`） |
| 卡比獸 | J / 小按鈕「死光」 | 破壞死光：蓄力、扇形掃射、硬直；可反覆擊中並擊退，也會打電球 |
| 卡比獸 | K / 小按鈕「冰拳」 | 急凍拳：蓄力、朝瞄準方向衝刺，打中的敵人被冰凍（不能動、不能用技能） |
| 噴火龍 | 空白鍵 / 主按鈕 | 飛行 `fly.duration` 秒：只是移動變快；打不打得到它看各技能的高度範圍（見下）對上 `fly.height`；冷卻從降落才開始 |
| 噴火龍 | J / 小按鈕「噴火」 | 在 `duration` 秒內噴出 `quantity` 團小火球，每團朝噴出當下敵人位置、再隨機偏 ±`random_offset_angle` 度；可邊走邊噴；火球有 HP，會被死光、落地震波、急凍拳、電球爆炸打掉；打到人就消失，不分高度 |
| 菊草葉 | 空白鍵 / 主按鈕「葉刃」 | 葉刃連斬：朝按住的方向跳 `jump_distance`（空中 `jump_duration` 秒、最高 `jump_height`，落地不傷人），接著甩葉子 `slashes` 下（每下 `slash_interval` 秒；第一下左上→右下、第二下右上→左下，輪流），每下朝敵人前進 `slash_step`（碰到身體就停），每下在揮到一半時打中半徑 `slash_radius` 內的人 `damage`；技能期間頭上的葉子放大 `leaf_scale` 倍（只影響畫面），並真的被甩過去當刀，原本的刀光留作淡淡的殘影；冷卻從起跳算；被冰凍或抓住會中斷 |
| 菊草葉 | J / 小按鈕「飛葉」 | 飛葉快刀：在身邊 `scatter_radius` 內隨機距離灑出 `count` 片樹葉（`Game.Balls` 裡 `K=ballLeaf`，`Wait` 時掛在原地）；各自隨機等 `min_delay`～`max_delay` 秒後朝當時敵人的位置以 `speed` 衝出，碰到就 `damage` 並消失；樹葉有 `leaf_hp`，會被死光、落地震波、急凍拳、電球爆炸打掉 |
| 菊草葉 | K / 小按鈕「藤鞭」 | 藤鞭（**瞄準操作和擺尾一樣**：按住 K／小按鈕時只有自己看到預覽扇形並選方向，放開才施放，輕點朝對手，見暴鯉龍那列；方向在施放當下就鎖定，蓄力時轉搖桿不會轉）：先蓄力 `charge_duration` 秒（藤鞭伸出來，地上出現警示扇形給雙方看，期間定身），再朝鎖定的方向同時揮出左右兩條藤鞭，各從 ±`sweep_angle` 度在 `sweep_duration` 秒內掃向 0 度會合；長 `length`、寬 `width`；一次揮擊只打一下，被打中的敵人 `bind_duration` 秒內不能走路、不能用位移技能（衝刺、跳躍、飛行、急凍拳、地球上投、葉刃連斬），其他技能照用（`Player.BoundT`）；揮擊後還要定身 `recharge_duration` 秒（藤鞭縮回）；冷卻從蓄力開始算 |
| 六尾 | 空白鍵 / 主按鈕「冰山」 | 冰山滑行（Frost Field，`vulpix.go` `stepVulpix`）：六尾踩著冰山滑 `spray_duration` 秒，速度 `slide_speed`、方向照方向鍵／搖桿（8 向），沒按就沿著朝向，**停不下來**（滑行中視為定身：`Player.Rode`，走路程式不介入，包括結束那一個 tick；放水與緩速不影響）；每次最後一步裁短，所以距離剛好 `slide_speed×spray_duration`。冰山大小與軌跡寬度都是 `radius`（腳下那塊冰山畫多大另有 `scale`，只影響畫面，乘在 `radius` 上，判定不變）：起點與路上每隔 `radius` 留一片冰（`Game.Frost`，存在 `duration` 秒，`maxFrost` 片上限，最舊的先丟），所以軌跡不會斷。身體中心站在冰上、高度在 `min_height`～`max_height` 內時，走路／飛行／空中操控速度：踩自己的冰 ×(1+`boost`)、踩對手的 ×(1−`slow`)，兩種都踩就相乘（`frostFactor`，和放水疊乘）；滑行時的速度不乘冰。**冰山碰到對手**（這一步滑過的線段離對手 ≤ `radius`＋對手半徑，且對手高度在帶內）就先受 `contact_damage`，然後爆炸；**時間到**也爆炸（沒有碰撞那一下）：`explosion_radius`（從冰山中心，加上對手半徑）內、高度在帶內的對手受 `explosion_damage`，發 `iceburst` 事件（`R`＝`explosion_radius`）；爆炸後滑行結束（兩段傷害規則同電球），並在爆炸點留下一片寬 `explosion_radius` 的冰（`Frost.R`，0＝軌跡寬度 `radius`；規則同軌跡：對手變慢、六尾自己加速；`explosion_radius` 為 0 就不留）。被冰凍、抓住、藤鞭綁住或麻痺會讓冰山直接消失（不爆）。滑行中不能開冰光線，冰光線施放中也不能開冰山（兩個都會定身）；冰風暴可以同時。冷卻從出發算 |
| 六尾 | J / 小按鈕「冰風暴」 | 冰風暴：風暴從六尾所在位置出發，以 `follow_speed` 追著敵人（和打雷的陰影同一套），每 `interval` 秒一擊、共 `strikes` 擊，範圍 `radius`（含敵人身體）、`damage`；同一次施放裡每打中第 `freeze_hits` 次（3、6…）就冰住敵人 `freeze_duration` 秒（和急凍拳同一個冰凍狀態與特效）；高度不在帶內的敵人打不到，但那一擊照樣用掉；不定身；施放期間不能再放，冷卻從施放算；每一擊也會對範圍內「對手的」投射物（電球、火球、飛葉，含還沒射出的葉子，以及對手的冰柱）各扣 `damage`，不管有沒有打到人、不看高度，自己的投射物不受影響 |
| 六尾 | K / 小按鈕「冰光線」 | 急凍光線：施放後 `control_duration` 秒內六尾定身（`casting`），方向鍵／搖桿改成移動光線落點（地面上的一點，速度 `speed`，離身體最遠 `length`，初始在瞄準方向的 `start_distance` 處：0 在六尾身上、越大離搖桿方向越遠、不能超過 `length`）；光線不傷人。落點經過的地方長出冰柱（`Game.Balls` 裡 `K=ballPillar`，半徑 `pillar_radius`、HP `pillar_hp`、`pillar_duration` 秒後融化）：沿落點這一步走過的路徑細取樣，離自己任何一根活著的冰柱不到 1.5 倍半徑就不長（掃回頭不會疊）；全場最多 `maxPillars` 根。**只擋對手**（高度在 `freeze_beam` 的 `min_height`～`max_height` 內），主人自己穿得過；擋法是移動結束後把重疊的身體推出去（`blockByPillars`）。冰柱和投射物同一套：被死光、落地震波、急凍拳、電球爆炸打掉血量；對手的電球／火球／飛葉碰到會讓冰柱扣自己的傷害、投射物當場用掉（電球下一步爆炸）；自己的投射物和還沒射出的葉子穿過。被冰凍會中斷噴霜與光線，施放中的冰風暴照跑。光線的嘴巴位置只在畫面上畫（朝向是客戶端的事），伺服器以身體中心量長度 |
| 木木梟 | 空白鍵 / 主按鈕「滑翔」 | 滑翔（Tailwind）（`rowlet.go` `stepRowlet`）：沿按住的方向（沒按就朝向）出發，以半徑 `radius`、速度 `speed` 走圓弧 `duration` 秒，轉向對手所在那一側；轉過的角度 = `speed×duration/radius`（不另設角度參數），每步用弦長精確走在圓上；出發後方向鎖定；不傷人、不受冰霜／緩速影響；施放中視為定身（`casting`，走路程式不介入），被冰凍會中斷；蓄龍捲風時不能滑翔，滑翔中不能開始蓄龍捲風；冷卻從出發算。**木木梟一直飛在 `fly_high`**：`Reset` 就把 `Z` 設成它，沒被抓住時每步以 `fly_high/flyRiseTime` 爬回去，所以只有 `min_height`～`max_height` 含這個高度的招式打得到它 |
| 木木梟 | J / 小按鈕「箭雨」 | 箭雨：一次射出 `count` 支箭（`K=ballArrow`），以對手方向為中心、相鄰 `angle` 度、速度 `speed`；碰到對手（半徑 `radius`、高度在 `min_height`～`max_height`）扣 `damage`、`SlowT` 設為 `slow_duration`（走路／飛行／空中操控速度 ×(1−`slow`)，與冰霜、放水疊乘）並用掉，沒打中的飛到牆邊；箭有 `hp`，和其他投射物互相扣對方傷害（同 `clashBalls`）；蓄／放龍捲風時不能射；冷卻從施放算 |
| 木木梟 | K / 小按鈕「龍捲風」 | 龍捲風（`radius` 是判定範圍，`scale` 只放大畫面上的漏斗，兩者分開調）：先定身蓄力 `charge_duration`，龍捲風（`K=ballTornado`，`hp`、半徑 `radius`）出現在木木梟瞄準方向前方（身體半徑＋龍捲風半徑處），再定身 `recharge_duration`（龍捲風已經放出，之後自己跑）；它像冰風暴一樣以 `speed` 追著對手、不會衝過頭，累計走 `distance` 或活滿 `duration` 秒就消失；冷卻從蓄力開始算。碰到對手（半徑＋對手身體、高度在帶內、對手沒被別的東西抓住）就把它**抓住**：龍捲風停在原地、`Life` 變成 `spin_duration`（旋轉剩餘時間），對手 `Held`＋`Twirl=twirlSpin`（不能動不能出招）、位置釘在龍捲風中心、高度在旋轉一半時間內升到 `lift_height`，每 `spin_interval` 秒受 `spin_damage`（`stepTwirl`，在投射物之後跑所以高度以它為準）。旋轉時間到或龍捲風被打壞（HP≤0，或被投射物對撞用掉）就 `twirlFall`：`twirlFallTime`（0.4 秒）內落地，落地受 `landing_damage` 並發 `slam` 事件（和卡比獸落地同一個特效，半徑＝龍捲風半徑）。龍捲風和投射物同一套會被死光、落地震波、急凍拳、電球爆炸、冰風暴打掉血量，有自己的 `mass`（同 `hyper_beam.mass`：死光／急凍拳比它重就依質量差推開它，被抓的人跟著動；不比它重就推不動），和對手投射物對撞時扣對方 `spin_damage`；和冰柱互相穿過（`clashBalls` 特例），但會把碰到的對手冰柱**捲上天**（`grindPillars`）：冰柱升到 `lift_height`（速度＝落下速度 `lift_height/twirlFallTime`）、跟著龍捲風一起移動，在天上時不擋人（`blockByPillars` 跳過 `Z>0`）；碰到當下扣 `spin_damage`，之後只要龍捲風還抓著它就每 `spin_interval` 秒再扣一次，直到冰柱壞掉；龍捲風消失或被打壞就放開，冰柱落回地面（`Ball.Z`、`Carried`）。龍捲風自己不受傷、自己的冰柱不受影響。被冰凍會中斷蓄力，已放出的照跑。被抓的是木木梟時，落地後會自己爬回 `fly_high` |
| 鯉魚王 | 空白鍵 / 主按鈕「彈跳」 | 彈跳（`magikarp.go` `stepMagikarp`，唯一的技能）：`duration` 秒內升到 `height`、在最高點停 `highest_point_duration` 秒再落下（升降各 `(duration−停留)/2` 秒，升的時候越來越慢、落的時候越來越快，所以高度與時間剛好是設定值，與 tick 長度無關）；空中可用方向鍵調整位置，前半段（到整段時間的一半）速度 `air_speed_first_half`、後半段 `air_speed_second_half`；高度在 0..`height`，所以只有打得到那麼高的招式碰得到它；落地不傷人；冷卻從起跳算。**每落地一次算一次**（`Player.Hops`），累計 `evolve_hops` 次那一下落地就進化（`evolve`：`Kind` 變 7，發 `evolve` 事件，HP 依「剩下的比例」換成暴鯉龍的上限）。跳到一半被抓住（`Held`，龍捲風、上投）就不算、由抓住它的人決定高度。被冰凍／擊暈時按鍵無效，但已經在空中的跳照跑。鯉魚王與暴鯉龍各有自己的 `max_hp`、`walk_speed`、`radius` |
| 暴鯉龍 | 空白鍵 / 主按鈕「海浪」 | 海浪（`stepGyarados` / `stepWaves`）：先定身跳起 `wave.jump_height`、`jump_duration` 秒，落地震動地面，發 `wave` 事件，並產生一道海浪（`Game.Waves`，只在伺服器，客戶端用事件自己畫）：從身體邊緣以 `speed` 往四周擴散到 `radius`；海浪的前緣**第一次**碰到對手身體時，對手高度在 `min_height`～`max_height`（預設 0～60）內就受 `damage`，只打一次（跳在空中就從底下過去，但海浪也用掉了）；冷卻從起跳算；被冰凍／擊暈會中斷跳躍 |
| 暴鯉龍 | J / 小按鈕「死光」 | 破壞死光：機制和卡比獸一樣（同一段程式，`Step` 裡那一塊是 `snorlax || gyarados`，用 `cfg.beamOf(kind)` 取數值；跳躍與冰拳只有卡比獸有），但**數值是自己的 `gyarados.hyper_beam`**（欄位同 `snorlax.hyper_beam`，預設與卡比獸相同，改一邊不影響另一邊）；`hi` 的 `gbeam` 與 `cooldown.gbeam` 是它的警示扇形與冷卻 |
| 暴鯉龍 | K / 小按鈕「擺尾」 | 擺尾（`Input.Tail`）：先蓄力 `charge_duration` 秒（`Player.TailT`，**定身**；0＝按下就打）再掃出扇形，蓄力時地上畫出**警示扇形**（`tlx`/`tly` 是按下那一刻鎖定的方向、`tail.length`/`angle` 是範圍，前端 `drawTailWarning`：顏色隨蓄力填滿、一圈環跑到邊緣就是打下去的瞬間），所以蓄力時轉搖桿不會轉扇形、對手走出扇形就不會被打到；被冰凍／擊暈／抓住會中斷蓄力；蓄力中不能跳海浪或放死光；冷卻從按下算、`tail` 事件在打下去那一刻才發（畫面上的尾巴在這時甩出去）。**瞄準操作（兩種模式，之後別的技能也會照這個做）**：按住「擺尾」鍵（K，或手機的小按鈕）時畫面只給自己看一個**預覽扇形**（虛線，`drawTailAim`，不送給伺服器、對手看不到）、這顆按鈕變成方向桿（手機拖曳按鈕；鍵盤按住 K 時 WASD 改成瞄準、不走路），**放開才施放**；放開時沒選方向（輕點，或拖曳不到死區 20px）就**朝對手**。施放時頁面送 `tail:true` 加上 `tx`/`ty`（`Input.CastX/CastY`，世界座標，任意長度，(0,0)＝朝對手；`Game.castDir`），維持約 120ms（`CAST_PULSE`）讓伺服器一定收得到，之後才是上面的蓄力＋給對手看的警示扇形。所以方向不再來自搖桿 `ax/ay` 或最後走的方向。頁面的 `AIMED`（角色→技能名）決定哪個角色的 K 技能是這種瞄準方式，目前是菊草葉的藤鞭和暴鯉龍的擺尾，要加別的技能就在 `AIMED` 和 `PAD` 的按鈕定義各加一筆、伺服器端那個技能改用 `castDir`。扇形：總角度 `angle`、距離 `length`（加對手半徑），對手中心在扇形內、高度在帶內就被往瞄準方向擊退 `knockback` 並擊暈 `stun_duration` 秒（`Player.StunT`：和冰凍一樣不能走也不能出招，進行中的衝刺與蓄力會中斷，但不會被畫成冰），發 `tail` 事件（`dx`/`dy` 是方向，為 0 時被省略，前端要 `|| 0`）；不能在跳躍或死光進行中用；冷卻 `cooldown` 從出招算 |
| 噴火龍 | K / 小按鈕「上投」 | 地球上投：朝瞄準方向衝 `length`，碰到（地面上的）敵人就抓住（`Held`，不能動不能用技能）、飛到 `high` 再摔下，落地受 `damage` 並播落地震波特效；噴火龍中途被冰凍會鬆手 |

技能判定都是「技能範圍 + 對方身體半徑 `radius`」，另外每個攻擊技能都有自己的**高度範圍**
`min_height`～`max_height`（含頭尾，寫在該技能的設定區塊裡；`Reach.hits`）：目標當下的高度 `Z` 在範圍內才打得到，
高於或低於都打不到。檔案裡的預設：地面技能（死光、落地震波、地球上投）是 0～12（`groundZ`），
電球與急凍拳是 0～60（打得到 `fly.height` 以內的飛行，打不到跳到 100 的卡比獸），打雷與火球 0～1000（等於不看高度）。
`DefaultConfig()` 的電球與急凍拳是 0～12，所以預設值的噴火龍飛行時它們打不到。
電球飛行中碰到敵人，先受 `contact_damage`，再受爆炸的 `explosion_damage`（兩者同一個高度範圍）。
**投射物互撞**：電球、火球、飛葉只要主人不同、飛行路徑相碰（半徑相加，不看高度），就互相扣對方的傷害（電球 `contact_damage`、火球 `damage`、飛葉 `damage`）。火球與飛葉血量歸零就當場消失，電球歸零則下一步爆炸（和被技能打掉一樣）。判定沿著整個 tick 的相對位移，快速的飛葉不會穿過火球；掛在菊草葉身邊還沒射出的葉子不參與。實作在 `clashBalls`，每對都用 tick 開始時的血量，結果與陣列順序無關。

## 設定檔 `config.json`

結構：`court.{mobile,web}`、`pikachu.{max_hp,walk_speed,radius,volt_tackle,thunder,electro_ball}`、
`snorlax.{max_hp,walk_speed,radius,ice_punch,jump,hyper_beam}`、`charizard.{max_hp,walk_speed,radius,fly,fire_breath,seismic_toss}`、`chikorita.{max_hp,walk_speed,radius,leaf_slash,razor_leaf,vine_whip}`、`vulpix.{max_hp,walk_speed,radius,frost_field,ice_storm,freeze_beam}`、`rowlet.{max_hp,walk_speed,radius,fly_high,tailwind,arrows,tornado}`、`magikarp.{max_hp,walk_speed,radius,evolve_hops,jump}`、`gyarados.{max_hp,walk_speed,radius,wave,tail_whip}`（暴鯉龍的死光是自己的 `gyarados.hyper_beam`）。每個欄位的意思看 `config.go` 的註解。

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
- 死鬥：每邊出場人數由房間的 `size` 決定（`{"size":3}` 或 `{"size":6}`，預設 6＝全部角色；比賽中不能改，改了大家重選，`lobby` 帶 `size`）。雙方各送一個出場順序 `{"order":[2,0,3,1,4,5]}`：6 隻時是 0..`kinds`-1 的排列，3 隻時是任選 3 隻不重複的角色，長度要等於 `size`；兩邊都送了就開打，一方有 `size` 隻都倒下就輸（快照 `tm.n` 是每邊人數）。
  一隻 HP 歸零就換順序中的下一隻（`room.boutOver` / `nextRound`，間隔 `roundReset` 2 秒），**活著的角色保留剩下的 HP**，
  位置與技能狀態重置；一方全部角色都倒下整場結束（兩邊同時倒下最後一隻 → slot 0，和單場同一個任意規則）。
- 對手的後續順序是秘密：快照的 `tm`（`c` 目前第幾隻、`f` 已倒下的角色、`o` 整場結束、`mw` 勝者）只公開已倒下的，
  自己的順序由前端自己記著。測試用 `room.step(now)`，不必開 socket（team_test.go）。

## 放水（讓分）

大廳的拖拉條，每格 10%、最多 90%（`easeStep` / `easeMax`，`{"ease":30}`）。放水的玩家**最大血量與移動速度**都乘 `(100-放水)%`
（`Game.Ease`、`Game.maxHP`；速度在 Step 的走路/飛行/空中操控那一處統一乘，技能的衝刺不受影響）。
放水只在開打前設定，對戰中拒絕；`Reset`、死鬥換隻都沿用；新加入的玩家從 0 開始。血條仍以角色滿血為基準，所以放水的人血條比較短。

## 單機（電腦對手）

大廳在沒有真人對手時多一排「單機：對皮卡丘 / 對卡比獸」按鈕（`{"bot":0|1}`，`{"bot":-1}` 改回等真人）。電腦佔住空的那個 slot
（`room.bot[i]`，`present(i)` = 真人或電腦），真人選好角色就開打；有第二個真人進房時電腦讓位；真人離開房間就刪掉（電腦不撐房間）。
單場對戰才有，死鬥拒絕（`setTeam(true)` 在有電腦時回 false）；大廳的 `lobby` 帶 `bot`（每個 slot 電腦玩的角色，沒有是 -1），`here` 把電腦算進去。

- **電腦只是替那個 slot 產生 `Input`**（`bot.go` `Bot.Decide`，`room.step` 每個 tick 在 `g.Step` 前呼叫，持有 `r.mu`），所以遊戲不知道誰在操作、
  電腦也做不出真人做不到的事。技能鍵只在做決定的那一個 tick 按下（`botEvery`＝4 tick 才決定一次，中間維持搖桿），否則皮卡丘的伏特攻擊會
  整串連發。
- **不會太笨也不會太神**：看對手與投射物是 `botReact`（5 tick≈0.17 秒）以前的樣子；有些招只有一定機率出（冰拳 50%、起跳 25%、貼身衝刺 50%）；
  冰拳會預判對手移動；繞圈方向每 0.8–2 秒隨機換。這些是常數，不在 `config.json`。
- **共用的部分**（`bctx`）：威脅偵測 `findThreat`（投射物最近距離與到達時間、死光扇形、急凍拳路線、卡比獸落地範圍、打雷陰影、伏特攻擊連段）、
  8 向搖桿 `dir8`、離牆、`rootedFor`（對手還要多久不能動：麻痺、冰凍、死光、蓄力電球、打雷中）。**每隻角色各一個函式**（`pikachu`、`snorlax`），
  只寫那隻的取捨；要新增角色就加一個函式、加進 `botKinds`、`Decide` 的 switch，並補 `findThreat` 還不認得的招式。
- 皮卡丘：留在對手射程外繞圈丟電球（對卡比獸射程外＝冰拳距離＋半徑；電球蓄力會定身，所以只在射程外丟）；威脅逼近就用伏特攻擊垂直閃，
  閃完用剩下兩次衝撞（8 向衝刺會算會不會打到）；對手被麻痺／冰凍／死光硬直就放打雷，但有東西打過來時不放（打雷定身近 3 秒）。
- 卡比獸：慢慢走近；球快到時起跳越過（球到達時間 0.25–0.9 秒內才跳，太晚跳不到高度 60 以上），起跳中朝對手飛，落地後 1 秒內接冰拳；
  對手被冰凍／麻痺／死光硬直時走到 150 以內放死光（越近被打中越多次）；被麻痺時不動（死光會讓自己多定身 3.8 秒）；皮卡丘衝刺連段進行中起跳反擊；
  打雷陰影來了就走開。
- **躲投射物、龍捲風、冰風暴**（`hazards` / `safeDir`）：箭、電球、火球、飛葉是直線飛的危險物，龍捲風與冰風暴是會追人的（`thChase`，離身體邊緣
  `botChaseZone` 以內就一直躲，不是只在快追到時才躲；追人期間也不蓄力）。躲的方向不是「垂直於最近那一顆」，而是把 8 個搖桿方向各往前演算
  `botHorizon`（1.2 秒），投射物直線飛、追人的追著當時的位置，挑離所有危險物最遠的方向，所以一排箭雨會走進縫裡。對手在瞄準箭雨（`ArrowT>0`）時，
  兩隻都不開會定身的招（電球、打雷、冰拳、死光）。
- 卡比獸的死光：冰拳打中（對手冰凍中）就一定放，不管距離：離得近就貼臉再放；來不及在對手解凍前走到 150 內，就站在原地放（死光長度內）；
  超出死光長度才先走近。
- `bot_test.go`：每個決策一個測試（場景凍結、直接看 `Decide` 的輸出），加上兩隻電腦打到分出勝負的模擬（用 `config.json` 的數字，
  確認不會卡死、勝負不是一面倒；目前皮卡丘對卡比獸約 3:1）。目前**只有皮卡丘、卡比獸**會玩，不認得其他角色的招式（對手是噴火龍、菊草葉等時，
  只會躲投射物與已知的招式）。
