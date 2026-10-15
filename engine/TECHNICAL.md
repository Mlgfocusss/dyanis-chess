> **Пути в этом документе** (`cmd/…`, `internal/…`, `assets/…`, `data/…`, `train/…`, `net-kingf-m4.bin`) отсчитываются от папки `engine/`: с 2026-10-15 движок лежит отдельно от сервера сайта. Команды из этого документа выполняйте из `engine/` (`cd engine`). См. [`README.md`](./README.md).

# Dyanis — техническая документация

Шахматный движок на Go. Это описание текущего устройства проекта —
без истории, кто когда что добавлял; только как это работает сейчас.
Пользовательский README (как запустить) — в [`README.md`](./README.md).

## Обзор

- `internal/` — весь движок, не публичный API (недоступен для импорта
  извне модуля).
- Три точки входа: `cmd/cli` (локальная отладка + `-play` + `-uci`),
  `internal/uci` (протокол UCI поверх stdin/stdout), `cmd/wasm`
  (JS-API `window.Dyanis` для `web/`, React-фронтенда).
- **Доска — битборды, не плоский массив.** `internal/board.Board`
  держит per-(тип фигуры, цвет) `Bitboard`-а как источник истины для
  генерации ходов/атак, плюс mailbox-массив `squares [64]Piece`
  (приватный) как вторичный, синхронно поддерживаемый кэш — нужен
  только для O(1) чтения "что стоит на этой клетке" (`PieceAt`) и для
  кода, которому реально нужен снимок всей доски разом (`SquaresArray`,
  SEE). `movegen`/`search`/`eval` работают только через методы `Board`
  (`Pieces`, `Occupied`, `OccupiedBy`, `PieceAt`, `SquaresArray`,
  `SetSquare`), не трогают ни `squares`, ни битборды напрямую.
- Слайдеры (слон/ладья/ферзь) — magic bitboards, O(1) lookup вместо
  ray-walk. Не-слайдеры (пешка/конь/король) — precomputed таблицы атак.
- Оценка — ручной эвал (`internal/eval`, Texel-тюнинг) или, по опции,
  NNUE (`internal/nnue`); всё про NNUE — данные, формат весов, обучение
  (`train/`), сборка с SIMD, результаты (первая сеть: +171 Elo против ручного
  эвала на 100 мс, ≈ 2800 по `UCI_Elo` на 500 мс), следующие шаги — в
  [`NNUE.md`](./NNUE.md).
- Генерация ходов **напрямую легальная** (pin-mask/checkers-mask), не
  generate-then-filter через make/unmake — кроме одного намеренного
  исключения (взятие на проходе, вскрывающее шах по горизонтали).

## Структура проекта

```
dyanis-chess/
├── go.mod
├── cmd/
│   ├── cli/
│   │   └── main.go              # локальный запуск: печать позиции, perft, -play (+ -book), -uci
│   ├── wasm/
│   │   ├── main.go               # JS-API window.Dyanis: newGame/makeMove/undo/getState/
│   │   │                         #   engineMove/searchMove(стейтлесс, для воркера)/loadBook/...
│   │   │                         #   game — один живой *board.Board + стек game.moves
│   │   │                         #   ([]moveRecord{move, undo}) для undo, НЕ массив снимков доски
│   │   ├── index.html            # smoke-test страница (НЕ финальный UI, см. web/)
│   │   ├── README.md             # как собрать и проверить wasm-сборку
│   │   ├── wasm_exec.js          # копия из установки Go (глобальный класс Go)
│   │   └── dyanis.wasm           # собранный бинарник — В GIT НЕ КОММИТИТСЯ, см. .gitignore
│   ├── match/
│   │   ├── main.go               # матч двух UCI-бинарников: W/D/L, Elo ± CI, SPRT по парам, -carry
│   │   │                         #   (см. раздел ниже)
│   │   └── stats_test.go         # тесты пентаномиальной статистики (среднее, дисперсия, знак LLR)
│   ├── spsa/
│   │   └── main.go               # SPSA-подбор 16 параметров поиска через cmd/match (состояние в
│   │                              #   spsa.json, продолжается с места остановки; см. раздел SPSA)
│   ├── gendata/
│   │   └── main.go               # самоигра → обучающие данные NNUE (шарды по 32 байта на позицию,
│   │                              #   продолжение после остановки, -dump, -evals); см. NNUE.md
│   ├── nnuetool/
│   │   └── main.go               # файлы весов NNUE: info, random (для замеров), eval по шарду
│   ├── profile/
│   │   └── main.go               # go run ./cmd/profile [-threads=N] [-budget=5s] [-fen="..."]
│   │                              #   [-out=cpu.prof] — CPU-профиль BestMoveTimedMT от заданной
│   │                              #   (по умолчанию стартовой) позиции на фиксированном времени,
│   │                              #   печатает узлы/счёт по каждой завершённой глубине;
│   │                              #   go tool pprof -top cpu.prof дальше
│   └── tune/
│       ├── main.go               # go run ./cmd/tune -mode=findk|error|verify|tune [-k=] [-threads=]
│       │                         #   [-epd=] [-step=] [-checkpoint=] [-resume] — Texel tuning
│       │                         #   (см. internal/eval/tunable.go и раздел ниже)
│       ├── tune.go               # покоординатный спуск: paramSlots, отжиг шага, чекпоинты на
│       │                         #   диск, печать финальных весов как Go-литералов
│       └── testdata/
│           └── quiet-labeled.epd # 725 000 размеченных позиций (публичный quiet-labeled датасет)
├── internal/
│   ├── board/
│   │   ├── board.go, board_test.go  # Piece, Square, Board (+ приватные поля: squares [64]Piece
│   │   │                            #   mailbox, bb bitboards, hash uint64, kingSq [2]Square,
│   │   │                            #   materialPST int), начальная расстановка; Copy() — для
│   │   │                            #   одноразовых веток без отката
│   │   ├── bitboard.go              # Bitboard, per-(тип,цвет) битборды, precomputed knight/king/
│   │   │                            #   pawn attack tables, Pieces/Occupied/OccupiedBy/PieceAt/
│   │   │                            #   SquaresArray/SetSquare
│   │   ├── magic.go                 # magic bitboards: RookAttacks/BishopAttacks/QueenAttacks —
│   │   │                            #   O(1) слайдер-атаки вместо ray-walk (числа найдены офлайн-
│   │   │                            #   поиском, независимо перепроверены полным перебором occupancy)
│   │   ├── lines.go                 # SquaresBetween[64][64] — клетки строго между двумя данными,
│   │   │                            #   для связок и (будущей) генерации блокировки шаха
│   │   ├── material_hook.go         # PieceValue-хук (регистрируется eval'ом) + инкрементальное
│   │   │                            #   сопровождение Board.materialPST в MakeMove/UnmakeMove/SetSquare
│   │   ├── fen.go                   # парсинг и сериализация FEN (+ начальный расчёт hash/bb/materialPST)
│   │   ├── move.go                  # Move, MakeMove(m) Undo / UnmakeMove(m, Undo) — мутируют
│   │   │                            #   доску на месте (рокировка/en passant/превращение,
│   │   │                            #   + инкрементальные hash/bb/materialPST, + кэш kingSq);
│   │   │                            #   симметрично MakeNullMove() NullUndo / UnmakeNullMove
│   │   └── zobrist.go, zobrist_test.go  # Board.Hash() — просто читает поле;
│   │                                    #   computeHashFromScratch/VerifyHash — для инициализации и тестов
│   ├── book/
│   │   └── book.go, book_test.go        # Load (с диска) / Parse (из байт, для браузера) Polyglot .bin,
│   │                                    #   Chain/Source: несколько книг цепочкой по приоритету;
│   │                                    #   записи отсортированы по ключу (Parse/New сортируют
│   │                                    #   стабильно, если пришли не по порядку) — Lookup бинарный
│   ├── eval/
│   │   ├── bishops.go            # бонус за пару слонов (Texel-tuned) — Bitboard.Count() по
│   │   │                         #   слонам, без скана
│   │   ├── eval.go, eval_test.go, eval_material_verify_test.go
│   │   │                         # материал (Texel-tuned) + вызов всех остальных слагаемых,
│   │   │                         #   gamePhase; verifyMaterialPST — сверка инкремента с нуля,
│   │   │                         #   гоняется по perft-дереву (живёт здесь, не в internal/perft —
│   │   │                         #   хук регистрируется только этим пакетом)
│   │   ├── king_safety.go        # пешечный щит + открытые линии рядом с королём (Texel-tuned),
│   │   │                         #   битборды
│   │   ├── mobility.go           # подвижность фигур (веса Texel-tuned) — attack-битборд минус
│   │   │                         #   свои минус под пешкой
│   │   ├── mopup.go              # "загони голого короля к краю" в выигранных эндшпилях —
│   │   │                         #   вне scope тюнинга (gated-порог, не гладкая функция)
│   │   ├── pawn_cache.go         # PawnCache — per-поиск кэш pawnStructureScore по битбордам пешек
│   │   ├── pawns.go              # сдвоенные/изолированные/проходные пешки (Texel-tuned),
│   │   │                         #   fileMask/passedMask
│   │   ├── pst.go                # piece-square таблицы (Texel-tuned, кроме короля — см. ниже),
│   │   │                         #   taper короля по фазе; init() регистрирует board.PieceValue-хук
│   │   └── tunable.go            # параметризованные offline-версии материала/PST/mobility/
│   │                              #   pawns/king-safety/bishop-pair (TuneParams,
│   │                              #   EvaluateWithTuneParams, Compute*) — только для cmd/tune,
│   │                              #   боевой Evaluate() их не вызывает; см. раздел ниже
│   ├── movegen/
│   │   ├── movegen.go             # GenerateLegalMoves — напрямую легальная генерация через
│   │   │                          #   Checkers()/Pins() (checkmask/pinmask), без make/unmake в
│   │   │                          #   цикле легальности, кроме взятия на проходе (см. ниже);
│   │   │                          #   GenerateLegalCaptures — только взятия, для quiescence;
│   │   │                          #   IsSquareAttacked/AttackersTo/Checkers/Pins
│   │   ├── pins_fuzz_test.go      # случайные позиции (ровно один король на цвет) — Pins() не
│   │   │                          #   должен разрешать то, что GenerateLegalMoves запрещает
│   │   ├── notation.go, notation_test.go # SAN: рендер и разбор ходов (Nf3, exd5, O-O, e8=Q#), GameLog
│   │   └── status.go, status_test.go     # GameStatus/GameStatusFromMoves (мат/пат/50-move rule/
│   │                                     #   insufficient material) + GameStatusWithHistory
│   │                                     #   (+ троекратное повторение, нужна история партии)
│   ├── nnue/
│   │   ├── network.go, accumulator.go, nnue_test.go  # сеть 768→H→1 с двумя перспективами и
│   │   │                            # выходными корзинами по числу фигур: формат весов DYNN v2
│   │   │                            # (Load/Parse/Marshal), Refresh/Update аккумулятора,
│   │   │                            #   Evaluate; см. NNUE.md
│   │   └── simd_amd64.go, simd_other.go, simd_test.go  # AVX2-версии циклов (только сборка с
│   │                                #   GOEXPERIMENT=simd на amd64), иначе простые циклы
│   ├── openings/
│   │   └── openings.go, openings_test.go  # стартовые позиции для cmd/match и cmd/gendata: прогулка
│   │                                # по книге (+ случайные ходы), случайные тихие ходы, чтение
│   │                                #   FEN/EPD; golden-тест на 1200 FEN
│   ├── trainingdata/
│   │   └── record.go, record_test.go  # кодек записи обучающих данных (32 байта: позиция + оценка
│   │                                #   + результат + флаги); формат — в NNUE.md
│   ├── perft/
│   │   └── perft.go, perft_test.go  # Perft()/Divide()/VerifyHashes()/VerifyBitboards() —
│   │                                #   эталонные числа: старт + Kiwipete
│   ├── search/
│   │   ├── opening_book.go, opening_book_test.go # BestMoveWithBook()/...MT(): книга —
│   │   │       #   приоритет перед поиском; *MT/*Shared-варианты — книга поверх Lazy SMP /
│   │   │       #   межходовой TT
│   │   ├── nnue.go, nnue_test.go  # NNUE в поиске: SetNetwork, стек аккумуляторов по ply,
│   │   │                     #   ленивое обновление (см. раздел «Статическая оценка: NNUE»)
│   │   ├── gamehistory_test.go  # повторения с учётом истории партии до корня
│   │   ├── tt.go             # sharedTT (сама хеш-таблица, lockless: XOR-проверка слота) +
│   │   │                     #   TranspositionTable (per-вызов состояние: killers/history/
│   │   │                     #   searchPath/pawnCache/nodes) + NewSearch() — общая
│   │   │                     #   хеш-таблица, свежее остальное: для Lazy SMP И для
│   │   │                     #   межходовой персистентности одинаково
│   │   ├── negamax.go        # сам negamax: check extensions, null-move pruning, razoring,
│   │   │                     #   futility pruning, Late Move Pruning, Internal Iterative
│   │   │                     #   Deepening, LMR/PVS, alpha-beta + repetition detection
│   │   │                     #   (searchPath)
│   │   ├── ordering.go       # move ordering: TT-ход/MVV-LVA/killers/history,
│   │   │                     #   insertionSortByScoreDesc
│   │   ├── quiescence.go     # quiescence (captures-only, не in check) + delta pruning
│   │   ├── search.go         # SearchInfo, BestMove*-точки входа (в т.ч. *TT/*MT/*Shared —
│   │   │                     #   Lazy SMP + тёплая между ходами TT), aspiration windows
│   │   ├── search_test.go, tt_test.go, bestmovemt_test.go, sharedtt_race_test.go,
│   │   │   tacticalsuite_test.go
│   │   │       # tacticalsuite_test.go — 21 позиция из WAC (Win At Chess), термометр силы
│   │   │       #   игры, не тюнинг; bestmovemt_test.go/sharedtt_race_test.go — Lazy SMP
│   │   │       #   (последний — конкретно под go test -race)
│   │   ├── see.go                       # SEE на битбордах (seeBoard — локальный мутируемый
│   │   │                                #   снимок [цвет][тип]Bitboard, не копия [64]Piece)
│   │   ├── info_test.go                 # SearchInfo/PV/mate-score/node-count тесты
│   ├── uci/
│   │   └── uci.go, uci_test.go          # UCI-протокол: uci/isready/position/go/quit,
│   │                                    #   стримит info depth/score/nodes/pv на каждой глубине
├── train/                       # обучение NNUE (Python + PyTorch, .venv в корне, не в git):
│   │                            #   data.py (загрузчик шардов, признаки), check_loader.py
│   │                            #   (сверка с gendata -dump), train.py (обучение 768→H→1),
│   │                            #   export.py (квантование, запись DYNN, сверка с nnuetool);
│   │                            #   подробности и результаты — NNUE.md; out/ — чекпойнты и логи
├── assets/                      # book.bin'ы (gm2001.bin, komodo.bin, performance.bin, ...) —
│                                 #   читает CLI/UCI с диска; для браузера копия нужна отдельно
│                                 #   в web/public/assets/ (см. ниже)
└── web/                         # React-доска
    ├── package.json, vite.config.js, index.html
    ├── public/
    │   ├── wasm/                # dyanis.wasm + wasm_exec.js — копируются сюда вручную
    │   ├── assets/               # book.bin'ы — копируются сюда вручную
    │   └── engine-worker.js     # отдельный Web Worker: своя копия wasm, только для поиска
    └── src/
        ├── main.jsx, App.jsx, App.css, index.css
        ├── engine/
        │   └── dyanisEngine.js  # обёртка: sync API основного потока (бухгалтерия партии) +
        │                        #   async API воркера (сам поиск, никогда не блокирует страницу)
        ├── hooks/
        │   └── useElementWidth.js
        └── components/
            ├── Board.jsx        # react-chessboard: drag&drop + клик-клик
            └── MoveLog.jsx
```

## `internal/board`

| Файл | Содержимое |
|---|---|
| `board.go` | `Piece`, `Color`, `Square`, `CastlingRights`, `Board`. `Board.Copy()` — глубокая копия (все поля — плоские значения, копируются вместе с `Board` автоматически). |
| `bitboard.go` | `Bitboard` (тип-обёртка над `uint64`), 12 битбордов по (тип, цвет), таблицы атак коня/короля/пешки, `Pieces`/`Occupied`/`OccupiedBy`/`PieceAt`/`SquaresArray`/`SetSquare`. |
| `magic.go` | Magic bitboards для слайдеров: `RookAttacks`/`BishopAttacks`/`QueenAttacks`. |
| `lines.go` | `SquaresBetween(a, b)` — клетки строго между `a` и `b`, если они на одной линии/диагонали. |
| `material_hook.go` | `PieceValue`-хук + `Board.materialPST` (инкрементальная сумма материал+PST по всем фигурам кроме короля). |
| `move.go` | `Move`, `MakeMove`/`UnmakeMove`, `MakeNullMove`/`UnmakeNullMove`. |
| `zobrist.go` | Polyglot-совместимый Zobrist-хеш. `Board.Hash()`, `VerifyHash()`. |
| `fen.go` | `FromFEN`/`ToFEN`, `StartFEN`. |

### Представление: битборды + mailbox

`Board` хранит:
- `bb` — 12 приватных `Bitboard` (по типу фигуры × цвету) + occupancy-битборды — источник истины для генерации ходов, атак, связок.
- `squares [64]Piece` — приватный mailbox-массив, синхронно поддерживаемый вместе с `bb` в каждой точке, где меняется доска (`MakeMove`/`UnmakeMove`/`MakeNullMove`/`SetSquare`). Нужен только для O(1) точечного чтения (`PieceAt`) и для кода, которому нужна вся доска разом (`SquaresArray()` — копия массива; используется, например, `SEE`, и раньше использовался `eval`, пока тот тоже не перешёл на битборды).

**`SetSquare(sq, p)` — единственный поддерживаемый способ вручную
поправить позицию** (тестовые хелперы, конструирование кастомных
досок) в обход `MakeMove`. Прямая запись в `squares` напрямую (поле
приватное, так и было изначально задумано, но раньше называлось
`Squares` и было экспортировано — при миграции переименовано в
`squares` именно чтобы закрыть эту дыру: любой прямой доступ снаружи
пакета `board` теперь не компилируется вообще, а не просто "не
рекомендуется") молча рассинхронизировала бы `bb`, оставляя `squares`
и битборды в противоречии — обнаружилось на практике как минимум один
раз (тестовый хелпер, расставлявший фигуры до перехода на
`SetSquare`), вызывало падение генерации ходов с паникой при индексации
по несуществующей клетке.

**`Board.KingSquare(c)` кэширует позицию короля**, а не сканирует все
64 клетки на каждый вызов. Кэш (`Board.kingSq`) поддерживается в
`MakeMove`/`UnmakeMove` при ходе королём (включая обе рокировки).
Кэш не считается источником истины: перед использованием `KingSquare`
сверяет, что клетка из кэша ДЕЙСТВИТЕЛЬНО всё ещё держит короля
нужного цвета, и если нет — откатывается на честный full-scan и сам
чинит кэш для следующих вызовов.

### Magic bitboards (`magic.go`)

Слайдеры (слон/ладья/ферзь) получают атаки за O(1): маска релевантной
occupancy на клетке, умножение на magic-число, сдвиг — индекс в
предпосчитанную таблицу атак для этой клетки и этого паттерна
занятости. Сами magic-числа найдены офлайн случайным поиском (не
частью движка) и независимо перепроверены вторым скриптом, перебравшим
все occupancy-подмножества для всех 64 клеток и подтвердившим ноль
коллизий — корректность самого движка при этом не зависит от доверия к
поиску: маски, "честные" ray-walk атаки для заполнения таблиц и
арифметика индексации целиком в Go, при любом плохом числе тесты
(`board`-пакета + `TestMagicSlidingMatchesRayWalk`, уже удалён после
подтверждения) поймали бы расхождение.

### Инкрементальные поля: `hash`, `bb`, `materialPST`

Все три поддерживаются одинаково — рядом с каждой мутацией доски в
`MakeMove`/`UnmakeMove`, а не пересчитываются с нуля на каждый вызов:

- **`hash`** — Zobrist, XOR точечно на изменившееся. `UnmakeMove`
  восстанавливает `HashBefore` снэпшотом (не обратным XOR).
- **`bb`** — `put`/`remove`/`move` на изменившиеся фигуры, зеркально в
  обе стороны (`MakeMove` и `UnmakeMove` оба явно вызывают эти методы,
  не снэпшот).
- **`materialPST`** — сумма `PieceValue(p, sq)` (хук из
  `material_hook.go`, реальные веса регистрирует `eval`) по всем
  фигурам КРОМЕ короля — `UnmakeMove` восстанавливает снэпшотом
  (`MaterialPSTBefore`), как хеш, а не обратным пересчётом. Король
  исключён намеренно: его позиционная ценность зависит от `phase`
  (глобальная величина, меняется при любом взятии/превращении фигуры
  где угодно на доске, не только при ходе самого короля) — `eval`
  считает king-term отдельно, на лету, при каждом вызове `Evaluate`
  (это дёшево — два табличных чтения от уже закэшированного
  `KingSquare`, не скан).

`NewInitialBoard`/`FromFEN` — единственные два места, которые сеют все
три поля с нуля (`computeHashFromScratch`/`rebuildBitboards`/
`rebuildMaterialPST`). `VerifyHash`/`VerifyBitboards`/
`VerifyMaterialPST` сверяют инкремент с честным пересчётом — гоняются
по perft-дереву в тестах, а не доверяются на слово.

**Move application — make/unmake, не копирование.** Стандартная
дисциплина вызова — как в стеке рекурсии:
```go
undo := b.MakeMove(m)
// ... смотрим на позицию, рекурсируем ...
b.UnmakeMove(m, undo)
```

**`Board.Copy()` остаётся в API** для мест, где ветка одноразовая и
её нечем/незачем откатывать, а исходную (часто ЧУЖУЮ, живую) позицию
трогать нельзя: `search.principalVariation`, `movegen.checkSuffix` в
`notation.go`. Любой код, который держит указатель на позицию "до
хода" и ожидает, что `MakeMove` где-то ещё её не тронет, обязан явно
взять `Copy()` заранее — само по себе это не гарантируется.

## `internal/movegen`

`GenerateLegalMoves` генерирует ходы **напрямую как легальные**, не
generate-then-filter. Для горячего пути есть версии без выделения памяти:
`AppendLegalMoves(b, dst)` и `AppendPseudoLegalCaptures(b, dst)` пишут в буфер
вызывающего (в `negamax` и `quiescenceBody` это массив на стеке; сверх его
размера `append` сам уходит в кучу), `HasLegalMove` работает на стековом
буфере, а связки считает `computePins` (маска связанных фигур + массив
допустимых клеток; `Pins()` — обёртка над ним для тестов). Порядок ходов тот
же, что у `GenerateLegalMoves`:

1. `Checkers(b)` — битборд фигур, держащих короля стороны-к-ходу под
   шахом (0, 1 или 2 — двойной шах разрешает только ход королём).
2. `Pins(b)` — для каждой связанной фигуры маска клеток, куда ей всё
   ещё можно ходить (линия до короля + сама связывающая фигура).
3. Ход короля — отдельная проверка на каждую клетку-кандидата
   (`squareAttackedWithOccupancy` с occupancy БЕЗ самого короля —
   иначе король "не видел бы", что не может отступить назад по линии
   шаха, которую сам же и блокировал).
4. Остальные фигуры — их обычный attack-битборд пересекается с
   `checkMask` (при шахе: взять шаховщика или заблокировать линию) и
   `pinMask` (если связана).

Единственное оставшееся исключение с реальным `MakeMove`/
`IsSquareAttacked`/`UnmakeMove` — взятие на проходе: снимает две пешки
с одной горизонтали разом, что может вскрыть шах по этой горизонтали
даже если ни одна из пешек по отдельности не была связана. Редкий
случай (максимум один кандидат на позицию), так что цена честной
проверки пренебрежима — то же решение принимает практически любой
битбордовый движок для этого конкретного случая.

`GenerateLegalCaptures` — отдельная генерация **только взятий**,
напрямую из битбордов (без прохода по тихим ходам вообще), для
`quiescenceSearch`, где подавляющее большинство узлов не под шахом и
интересуются только взятиями. **Не заменяет** `GenerateLegalMoves` для
проверки мата/пата — пустой результат означает только "нет взятий", не
"нет ходов вообще".

Корректность генератора проверяют perft-тесты с эталонными числами и
`pins_fuzz_test.go`. Старый make/unmake-генератор, служивший эталоном для
сверки множеств ходов, удалён вместе с `VerifyLegalMoveGeneration`.

`IsSquareAttacked`/`AttackersTo` — табличные/magic lookup'ы по всем
типам фигур; `AttackersTo` — то же самое, но возвращает битборд всех
атакующих, а не bool (нужен `Checkers`/`Pins`).

`GameStatus`/`GameStatusWithHistory`/`InsufficientMaterial` — без
изменений в этой миграции (см. прошлую версию документа за деталями,
не менялись).

## `internal/eval`

Материал + piece-square tables (`pst.go`, тейпинг midgame/endgame по
фазе — только для короля) + mobility + пешечная структура + king
safety + пара слонов + mop-up. Все термы читают битборды через
`Board.Pieces`/`Occupied`/`OccupiedBy` — ни один больше не сканирует
64 клетки вручную.

Все константы/таблицы, кроме king PST и mop-up, — Texel-tuned
(`cmd/tune`, см. подраздел ниже), не подобраны вручную.

**Материал+PST — почти целиком инкрементальный.**
`materialAndPstScore` — это `b.MaterialPST()` (одно чтение поля,
поддерживается в `board`) плюс king-term, посчитанный на лету по
текущей `phase`.

**`PawnCache` (`pawn_cache.go`)** — per-поиск кэш `pawnStructureScore`,
ключ — микс двух битбордов пешек (не позиция целиком: пешечная
структура одна и та же для огромного числа узлов дерева, где ходят не
пешки). Живёт внутри `search.TranspositionTable` (один кэш на один
вызов `BestMove`/`BestMoveTimed`), **не** как глобальная переменная
пакета — глобальный кэш потребовал бы мьютекса в момент, когда поиск
станет многопоточным, а так каждый поиск просто получает свой.
`Evaluate(b, cache)` принимает `cache` как `nil`-safe параметр — `nil`
означает "без кэша", не ошибка.

### Тюнинг эвала (`internal/eval/tunable.go`, `cmd/tune`)

Веса эвала (кроме king PST и mop-up — ниже, почему) подобраны Texel
tuning: покоординатный спуск по 341 параметру (материал + PST +
mobility + пешечная структура + king safety + пара слонов) на 725 000
размеченных позиций (`cmd/tune/testdata/quiet-labeled.epd`), минимизируя
MSE между `sigmoid(K·score/400)` и реальным исходом партии.

`tunable.go` — параллельный, чисто аддитивный набор функций рядом с
боевыми: `MaterialPSTParams`/`MobilityParams`/`PawnStructureParams`/
`KingSafetyParams` + `TuneParams` (их сумма, плюс `BishopPair int`) —
явные структуры весов вместо константных выражений. `Compute*` (
`ComputeMaterialPST`, `ComputeMobilityScore`, `ComputePawnStructureScore`,
`ComputeKingSafetyScore`, `ComputeBishopPairScore`) — те же формулы, что
боевые `materialAndPstScore`/`mobilityScore`/`pawnStructureScore`/
`kingSafetyScore`/`bishopPairScore`, только читают вес из `params`, а не
из пакетных констант. `EvaluateWithTuneParams(b, params)` — их сумма +
king-term + mop-up, тот же результат, что `Evaluate()`, при
`params == DefaultTuneParams`. Ни одна из этих функций не вызывается из
боевого `Evaluate()` — существуют только ради `cmd/tune`.

**`cmd/tune` не трогает `internal/eval` сам** — печатает финальные веса
как Go-литералы, перенос в `eval.go`/`pst.go`/`mobility.go`/`pawns.go`/
`king_safety.go`/`bishops.go` всегда ручной, отдельным, видимым шагом
(та же дисциплина, что и везде в проекте). Четыре режима:
`-mode=findk` (калибрует `K` сигмоиды), `-mode=error` (считает MSE при
текущих весах — журнальное число для сравнения), `-mode=verify`
(сверяет закэшированный путь `cmd/tune` с честным полным пересчётом на
намеренно сдвинутых весах — не только на дефолтных, где совпадение
тривиально), `-mode=tune` (сам покоординатный спуск: `+step`/`-step` на
train-выборке, шаг отжигается вдвое на каждой эпохе без улучшений до
`floor=1`, held-out MSE меряется отдельно только для контроля
переобучения, чекпоинт на диск после каждой эпохи, `-resume`
подхватывает прерванный прогон).

**King PST (`kingMidgamePST`/`kingEndgamePST`) сознательно вне
тюнимого scope** — король занимает лишь горстку клеток в реальном
миттельшпиле, у большинства клеток таблицы почти нет данных в
выборке; два независимых прогона с этими таблицами в scope сошлись к
одному и тому же пространственно бессвязному, экстремальному
паттерну (перепроверенная подгонка под шум, не сигнал) — см. `plan.md`
для истории. Таблицы остаются исходными, рукописными. Mop-up вне
scope по другой причине — gated-порог (`mopupEligible`), не гладкая
функция, для которой имеет смысл пробовать `+step`/`-step`.

## `internal/perft`

`Perft(b, depth)`/`Divide(b, depth)` — брутфорс-подсчёт листьев дерева
легальных ходов. `VerifyHashes`/`VerifyBitboards` идут по тому же
дереву и на каждом узле сверяют `Board.VerifyHash()`/`VerifyBitboards()`
с честным пересчётом. Эталонные числа — стартовая позиция и Kiwipete.

(`VerifyMaterialPST` — тот же приём, но живёт не здесь, а в
`internal/eval`, там, где регистрируется нужный ему хук — см. этот
раздел.)

## `internal/search`

Файл `search.go` (изначально ~1700 строк) разложен по темам на пять
файлов одного пакета — Go не различает, из какого файла символ внутри
пакета, так что сама по себе разбивка не меняла ни одной строки
логики:

| Файл | Содержимое |
|---|---|
| `tt.go` | `sharedTT` + `TranspositionTable` (детали ниже) |
| `negamax.go` | сам `negamax`: все эвристики поиска |
| `ordering.go` | move ordering |
| `quiescence.go` | `quiescenceSearch`/`quiescenceLoop` + delta pruning |
| `search.go` | `SearchInfo`, все точки входа `BestMove*`, aspiration windows |

### `TranspositionTable`: общая хеш-таблица + per-вызов состояние

Разнесены на два уровня:

- **`sharedTT`** — собственно хеш-таблица: `entries []ttSlot`, по
  умолчанию `ttSize = 1<<20` слотов (16 МБ; UCI-опция `Hash` и
  `NewTranspositionTableMB` задают другой размер, округляя вниз до степени
  двойки — индексация `hash & mask`, не modulo). Хеш 128 МБ против 16 МБ на
  500 мс силу не повысил (−8.1 ± 16.8 Elo), поэтому по умолчанию 16 МБ. **Без блокировок:** слот — два атомарных `uint64`, `data`
  (упакованная запись: score 24 бита, depth 8, ход 18, bound 2, gen 8) и
  `check = hash ^ data`. `probe` принимает слот, только если
  `check ^ data == hash`; разорванное чтение (две записи разных потоков
  перемешались) выглядит как промах, а не как неверный ответ. Слот 16
  байт (раньше запись 48 байт и мьютексы по шардам), таблица 16 МБ.
  Слотов меньше, чем возможных хешей — несовпадение хеша — честный
  промах кэша. Замещение — depth-preferred: чужая (другой хеш) запись
  с бо́льшей глубиной сохраняется вместо более мелкой новой; свежий
  результат той же позиции или пустой слот — всегда перезаписывается.
- **`TranspositionTable`** — всё, что специфично для ОДНОГО вызова
  поиска: `killers`, `history` (`[64*64]int`, не `map`), `searchPath`
  (repetition detection, ниже), `pawnCache`, `nodes`. Указывает на
  `*sharedTT` через поле `shared`.
- **`NewSearch()`** — метод `TranspositionTable`, возвращает НОВЫЙ
  `*TranspositionTable`, который делит `shared` с исходным, но
  начинает с чистых `killers`/`history`/`searchPath`/`nodes`. Один и
  тот же приём используется для двух разных целей: Lazy SMP (каждый
  поток свой `NewSearch()`, все — одна хеш-таблица) и межходовая
  персистентность (UCI-сессия/CLI `-play` держат один
  `*TranspositionTable` на всю партию, каждый `go`/ход берёт свежий
  `NewSearch()` от него).

**Возраст записей (`gen`).** Каждая `ttEntry` помнит поколение поиска
(`sharedTT.gen`), в котором записана. Поколение растёт один раз на ход:
в `NewSearch()` и в начале `bestMoveTimedMT`. В `store` depth-preferred
правило действует только против записи ТЕКУЩЕГО поколения; запись с
прошлых ходов вытесняется любой новой. Без этого глубокие записи давно
ушедших позиций навсегда занимали слоты в таблице, живущей всю партию.
Измерено: на 100 мс эффекта нет (−2 ± 15 по двум сериям, таблица не
заполняется); на 500 мс +14.6 ± ~17 Elo на 856 партиях (две серии,
SPRT не закрылся, нижняя граница ≈ −2.5) — принято вручную как
теоретически верное и безвредное, с положительным знаком в обеих сериях.

### Move ordering, null-move pruning, расширение на шах

Без изменений по сути: TT-ход → взятия по MVV-LVA → killer moves →
history heuristic, финальная сортировка — прямая вставочная
(`insertionSortByScoreDesc`), не `sort.Sort`. Null-move pruning — с
защитой от цугцванга (`hasNonPawnMaterial`) и от срабатывания в корне
(`isRoot`). Расширение на шах — на 1 ply, бюджет `checkExtLeft`
(`maxSearchCheckExtensions = 6` на линию), шахующие ходы исключены из LMR.

Null-move пробуется только при `staticEval >= beta` (стандартное условие:
иначе проба почти никогда не срабатывает, а узлы тратит). Измерено матчем
на 100 мс, две серии по 600 партий (seed по умолчанию: +16.2 ± 20.8,
`-seed=2`: +18.0 ± 21.7); в сумме +378 =503 -319, ≈ +17 ± 15 Elo.
Принято при нижней границе чуть выше нуля (по оценке суммы; `cmd/match`
серии не складывает). На 500 мс подтверждено: две серии по 600 партий
(+174 =277 -149 и +196 =255 -149), в сумме +370 =532 -298, ≈ +21 ± 15 Elo.

### Статическая оценка узла — один раз

`negamax` считает `eval.Evaluate` лениво и не более одного раза на узел
(`getStaticEval`): RFP, null-move, razoring и futility видят одну и ту
же позицию. Раньше каждый приём звал `Evaluate` сам, до четырёх раз за
узел. Поиск не изменился (узлы на фиксированной глубине совпадают до
единицы), время до глубины 17 на Kiwipete 10,12 с → 9,70 с (≈ +4,3%
скорости). Профиль после этого ровный: `Evaluate` ≈ 20% (из них
`mobilityScore` ≈ 6%), генерация ходов ≈ 19%, quiescence ≈ 28%,
`MakeMove` ≈ 11%, TT `probe` ≈ 10%.

### Repetition detection (`searchPath`/`isRepetition`)

(Исторический абзац; про историю партии до корня — ниже.) Закрывает
реальный пробел: раньше троекратное повторение проверялось
только `movegen.GameStatusWithHistory` **снаружи** дерева поиска
(CLI/UCI/wasm ведут историю партии сами) — `negamax`/
`quiescenceSearch` о повторениях **внутри собственной линии поиска**
ничего не знали (например, не видели, что какая-то форсированная линия
зацикливается в позицию, пройденную этим же поиском раньше).

`TranspositionTable.searchPath []uint64` — хеш позиции на каждом `ply`
текущей линии поиска, растёт по требованию. `recordPathHash`
записывает текущий узел; `isRepetition(ply, hash)` проверяет **строго
более мелкие** индексы (`< ply`) — то, что депth-first обход
гарантированно оставил предками именно этой ветки, без явного
push/pop при возврате. Проверяется **до** пробы TT в `negamax` — иначе
закэшированный результат с ДРУГОЙ линии мог бы замаскировать
повторение именно на этой. Тот же массив и та же проверка — в начале
`quiescenceSearch`, специально ради вечного шаха: он часто целиком
разыгрывается в «под шахом» ветке quiescence, не заходя в основной
move loop `negamax`. Возврат `0` при повторении не может замаскировать
мат: если тот же хеш уже встречался раньше на линии и поиск тогда
продолжился — значит, тогда это не был мат/пат, а раз позиция
идентична — не мат и сейчас.

**История партии до корня.** `TranspositionTable.SetGameHistory(hashes)`
передаёт поиску хеши всех позиций партии, включая текущую; хранятся все,
кроме последней (корня). `NewSearch()` и потоки Lazy SMP получают их
вместе с таблицей. `isRepetition` продолжает линию поиска в прошлое партии:
отрицательные «полуходы» −1, −2, … — это позиции партии перед корнем, и
возврат в любую из них (в пределах окна по `halfmoveClock`) — ничья, 0.
Достаточно одного возврата, как и внутри дерева: соперник может повторить
снова. Корень (`ply == 0`) не проверяется — там поиск обязан вернуть ход.
Так ничья сравнивается с оценкой: сторона с перевесом избегает повторения и
играет на победу, сторона в худшей позиции идёт в повторение как в спасение.
Раньше поиск видел только свою линию, и сторона с перевесом в +5…+9 пешек
спокойно шла в троекратное повторение (самоигра `cmd/gendata`, глубина 9,
46 партий: 37 ничьих повторением, после правки 9, все — при оценке 0).
Историю передают UCI (`session.history`), `cmd/cli -play`,
`cmd/wasm engineMove` и `cmd/gendata`. Не передаёт `cmd/wasm searchMove`
(веб-воркер получает только FEN) — для этого фронтенду нужно отдавать
историю ходов. Без истории (голый FEN) поиск не изменился: узлы до единицы.
Тесты: `TestIsRepetitionSeesGameHistory`,
`TestSearchAvoidsGameRepetitionWhenWinning`. Измерено против прежней
версии: 100 мс, `gm2001.bin`, две серии (seed 1–2, один `-carry`), SPRT 0/10
принял H1 на 551 паре: +303 =557 −242, **+19.3 ± 14.1 Elo** (по парам),
нижняя граница ≈ +5,2. Первая серия отдельно: +16.8 ± 18.9 на 300 парах.
Точечная оценка может быть завышена ранней остановкой. На 500 мс не
проверено.

`isRepetition(ply, hash, halfmoveClock)` смотрит только позиции после
последнего необратимого хода: индексы от `ply − halfmoveClock` до `ply − 1`
(отрицательные — из истории партии), потому что пешечный ход или взятие
нельзя отменить, и ничто до него повториться не может. Результат тот же (узлы идентичны), проход по
линии короче. `movegen.InsufficientMaterial` на каждом узле quiescence
сначала проверяет битборды пешек, ладей и ферзей (любой — выход `false`) и
только потом считает слонов и коней, без скана 64 клеток; вместе с
окном повторений это ≈ −3% времени, заметнее в эндшпилях.

### Остановка итеративного углубления на мате

В `runIterativeDeepening` после завершённой глубины, если счёт — собственный
мат (`score >= mateThreshold`) и `depth >= 2·(MateScore − score)` (длина
мата в полуходах, с запасом на обрезки, которые не гарантируют полный
перебор), цикл выходит. Раньше на «мат в 1» тратился весь бюджет времени.
При собственном проигрыше остановки нет (глубже ещё может найтись защита).
Фиксированная глубина без `movetime` (`go depth N`, один поиск) эту ветку
не затрагивает. Измерено: на мате в 1 ответ за 84 мс вместо 3 с; матч 100
мс, 150 пар: −4.6 ± 24.9 Elo (нейтрально).

### Razoring

`razorMaxDepth = 3`, `razorMargin` — на малой оставшейся глубине, не в
корне, не под шахом, не рядом с mate-границами: если `staticEval +
razorMargin[depth] < alpha`, узел не проверяется полным поддеревом
вслепую — сначала подтверждается нулевым окном через `quiescence(alpha
- 1, alpha)`, и только если quiescence САМ подтверждает, что даже
взятия/уклонения не спасают, весь узел обрезается в счёт из
quiescence. Симметричен futility pruning, но режет весь узел целиком,
а не отдельный ход — поэтому margin шире на каждой глубине.

### Futility pruning

`futilityMaxDepth = 3`, `futilityMargin[depth]`. Тихий ход (не взятие,
не превращение, сам не ставит шах), для которого даже `staticEval +
futilityMargin[depth]` не достаёт до `alpha`, отбрасывается без
рекурсивного вызова. Не применяется к первому ходу в списке (`i == 0`).

### Late Move Pruning (LMP)

`lmpMaxDepth = 3`, `lmpMoveCountThreshold[depth]` — move-count
аналог futility: не смотрит на eval вообще, просто на позицию хода в
уже отсортированном списке. После `lmpMoveCountThreshold[depth]`-го
кандидата на этой малой оставшейся глубине оставшиеся тихие
(не-шах, не-взятие, не-превращение) ходы отбрасываются целиком, без
поиска — move ordering уже исчерпало свои лучшие догадки к этому
месту в списке.

### Internal Iterative Deepening (IID)

`iidMinDepth = 5`, `iidReduction = 2` — на противоположном от
futility/razoring/LMP конце спектра глубин: применяется не близко к
листьям, а когда глубины ЕЩЁ много. Если в узле нет хода из TT (не
нашли/вытеснили) и `depth >= iidMinDepth`, гоняется `negamax` на этой
же позиции на `depth - iidReduction`, и TT перечитывается — `negamax`
и так безусловно сохраняет в TT лучший найденный ход в конце любого
вызова, отдельного кода для «сохранить результат IID» не нужно.

### Reverse futility pruning

`rfpMaxDepth = 6`, `rfpMargin = 100`: в узле с нулевым окном
(`beta-alpha == 1`), не в корне, не под шахом, не у границ мата, если
`staticEval - rfpMargin*depth >= beta` — узел режется без поиска.
Измерено матчем: +52.9 ± 25.8 Elo (470 партий, 100 мс/ход, SPRT
принял H1).

Суммарная проверка трёх приёмов ниже (RFP + SEE-pruning + LMR по
логарифму) против версии без них (с остановкой по времени): на
500 мс/ход +135.2 ± 53.8 Elo, 116 партий, SPRT принял H1 — прирост на
длинном контроле не меньше, чем на 100 мс.

### History со штрафами и гравитацией

`TranspositionTable.history` — теперь ограничена `±maxHistory (8000)`
(ниже `killerBand`, раньше могла переполнить свою полосу ordering'а).
`updateHistory` — обновление с «гравитацией»: `h += bonus - h*|bonus|/
maxHistory`, `historyBonus(depth) = min(8*depth², 1600)`. Тихий ход,
давший отсечку, получает бонус; тихие ходы, полностью просмотренные
на этом узле ДО отсечки (`quietsTried`), — такой же штраф со знаком
минус. Измерено: +22.9 ± 15.7 Elo на 1048 партиях (две серии), LLR ≈
2.8. Даёт возможность ниже использовать history для отсечений.

### Проигрывающие взятия — в конец списка

В `moveOrderingScore` взятие с `SEE < 0` (`losingCapture`) получает
`badCaptureBand = -captureBand` + MVV-LVA, то есть идёт ПОСЛЕ всех тихих
ходов (history ограничена ±8000, худший офсет MVV-LVA около −19000).
Остальные взятия — как раньше, выше killers. SEE зовётся только для
простого взятия, где жертва дешевле нападающего (иначе взятие заведомо не
проигрывает); en passant и взятие с превращением проигрывающими не считаются.
Побочный эффект: у тихих ходов индекс `i` в списке меньше, поэтому
LMR/LMP/futility срабатывают у них на другом месте. Измерено:
`-openbook=assets/gm2001.bin`, 100 мс, три серии по 300 пар
(seed 1–3, один `-carry`), в сумме +462 =939 −399, **+12.2 ± 11.0 Elo**
(по парам), нижняя граница ≈ +1.2; SPRT 0/10 не закрылся (LLR 2.30 из
2.94). Принято по правилу плана. На 500 мс не проверено.

### Killers по ply

Killer-ходы хранятся по расстоянию от корня (`ply`), а не по оставшейся
глубине, как в стеке поиска Stockfish: с LMR и расширениями узлы с одной
оставшейся глубиной лежат в разных частях дерева, и killers смешивались.
Измерено: `-openbook=assets/gm2001.bin`, 100 мс, две серии по 300 пар
(seed 1–2, один `-carry`), +280 =656 −264, **+4.6 ± 12.9 Elo** — в
пределах шума, SPRT не закрылся (LLR −0.08). Оставлено как стандартная
схема без замедления и усложнения, а не как доказанный выигрыш.

### Параметры поиска и SPSA (`tunables.go`, `cmd/spsa`)

Шестнадцать порогов поиска вынесены в `search.Tunables()` и открыты как
UCI-опции (`setoption name RfpMargin value 90` и т. д.): RFP и SEE-pruning,
futility/razoring/LMP по глубинам, константы LMR, бонус history, окно
aspiration. `cmd/match` принимает `-aopts`/`-bopts` (`Имя=значение,...`),
`cmd/spsa` подбирает их одновременно играя короткие матчи «плюс против
минус» (16 пар на итерацию, состояние в `spsa.json`, продолжается с места
остановки). 1000 итераций (≈ 32 000 партий, 100 мс) дали среднее по
второй половине прогона; против прежних значений на сборке того времени:
+20.9 ± 14.3 Elo (SPRT 0/10 принял H1). Перепроверено на сборке после
ускорений (`movePicker`, ленивая генерация): +305 =638 −257, **+13.9 ±
13.2 Elo**, нижняя граница ≈ +0,7. Приняты как новые дефолты:
AspirationDelta 50→41, Futility1/2/3 150/300/500→135/245/571,
Razor1/2/3 300/500/900→235/602/943, Lmp1/2 8/12→5/11, RfpMargin
100→95, SeePruneMargin 100→94, LmrDiv 225→223, HistBonusMul 8→7,
HistBonusCap 1600→1654 (остальные без изменений). Подбор шёл на 100 мс.
Проверка на 500 мс (одна серия 300 пар, новые значения против старых,
`-concurrency=8`): +130 =374 −96, **+19.7 ± 16.5 Elo**, нижняя граница
≈ +3,2 — значения годятся и на длинном контроле.

### Поэтапная выдача ходов (`movePicker`)

`newMovePicker` ничего не считает: он только запоминает вход. Первый
`next()` отдаёт ход из TT (если он есть в списке; он поворачивается в начало
с сохранением порядка остальных), а оценка, SEE и сортировка групп
(`build`) выполняются на втором вызове. Узлы, отсечённые ходом из TT, этих
затрат не несут. Порядок ходов не изменился (узлы совпали до единицы), время
до глубины ≈ −1,8%.

`negamax` берёт ходы через `movePicker.next()` вместо полной сортировки в
`orderMoves`. Группы выдаются в порядке: ход из TT, хорошие взятия
(MVV-LVA), killers, тихие ходы, плохие взятия (SEE < 0). Взятия и killers
сортируются сразу, а тихие ходы получают оценку по history и сортируются
только когда поиск дошёл до них (`sortQuiets`). Полосы оценок не
пересекаются, поэтому порядок внутри групп тот же, что у полной устойчивой
сортировки; это проверено: с немедленной сортировкой тихих узлы совпали с
прежними до единицы на 12 позициях. Реальное отличие одно: history читается
после поиска ходов из TT и взятий, то есть свежее. Узлов на бенчмарке
меньше (8 225 874 → 6 958 338), время до глубины ≈ −17%. Вместе с заменой
`map` на массив в `pieceOrderingValue` (≈ −3% сама по себе).
Измерено против предыдущей версии: `-openbook=assets/gm2001.bin`, 100 мс,
две серии по 300 пар (seed 1–2, один `-carry`), +317 =625 −258,
**+17.1 ± 13.8 Elo** (по парам), нижняя граница ≈ +3,3; SPRT не закрылся
(LLR 2.46). Это совокупный эффект скорости и более свежего порядка ходов.
Принято по правилу плана. На 500 мс не проверено. Прежние
`orderMoves`/`moveOrderingScore` удалены как мёртвый код.

### Кеш статического эвала

`TranspositionTable.evaluate` оборачивает `eval.Evaluate` в прямой кеш на
поток: `1<<16` записей по 16 байт, ключ — полный Zobrist-хеш, выделяется при
первом обращении. `Evaluate` зависит только от позиции, поэтому попадание
точное; одну и ту же позицию поиск видит через транспозиции и в разных
итерациях углубления. Используется в `negamax` (`getStaticEval`) и в
stand-pat quiescence. Оценки и узлы идентичны (12 позиций, 8 225 874 узла),
время до глубины ≈ −2…3,5% (в среднем около −3%). Кеш `1<<18` лучше не стал.

### Статическая оценка: NNUE (`nnue.go`)

`tt.evaluate(b, ply)` считает `eval.Evaluate`, а если поиску дана сеть
(`search.SetNetwork`, глобально, между поисками; по умолчанию `nil`) — сеть из
`internal/nnue`. Кеш оценок по хешу общий для обоих. Каждая
`TranspositionTable` в начале поиска (`runIterativeDeepening`,
`BestMoveInfoTT`) вызывает `nnueReset(root)`: запоминает текущую сеть (поиск
никогда не смешивает две; при смене сети сбрасывает кеш оценок и стек) и
считает аккумулятор корня. Таблица, которую никто не сбросил, оценивает
ручным эвалом; `nil`-таблица — тоже.

Стек аккумуляторов по `ply` (`accStack`): после `MakeMove` в цикле `negamax`,
в `quiescenceLoop` (после проверки легальности) и после null-move поиск
только записывает ход и `Undo` (`nnuePush`/`nnuePushNull` — пара присваиваний,
без сети ничего). Аккумулятор строится лениво в `nnueEvaluate`: от ближайшего
готового предка по линии ходы проигрываются вперёд (`nnue.Update`, для
null-move — копия). Запись на уровне `k` сбрасывает готовность этого уровня,
поэтому всё, что ниже текущего `ply`, всегда относится к текущей линии.
Отсечённые, обрезанные futility/LMP и взятые из кеша позиции аккумулятор не
считают вовсе. Тестовый флаг `nnueVerify` сверяет каждую оценку с пересчётом с
нуля (`nnue_test.go`). Без сети поиск не изменился: узлы до единицы, время в
пределах шума (Kiwipete глубина 15, три чередующиеся пары). Подробности,
форматы и замеры — в `NNUE.md`.

Корзины короля (формат весов v3): входы зависят от клетки своего короля
перспективы (`KingMap`, при `Mirror` — с отражением по файлам, король на
a–d). Ход короля в другую корзину или через линию отражения делает
пересчёт аккумулятора этой перспективы с нуля, для чего `nnuePush` на таком
ходу (`Network.KingRefresh`) сохраняет биты фигур позиции после хода
(`accEntry.snap`). Остальные ходы обновляются инкрементально, как раньше.
Лучшая сеть — `net-kingf-m4.bin` (4 корзины, общая часть весов).

**Сеть.** Архитектура `768 → H → 1` с двумя аккумуляторами (по перспективе
каждой стороны), активация SCReLU (`clamp(x, 0, QA)²`), квантование
`QA = 255`, `QB = 64`, `Scale = 400`. Выходной слой — **8 корзин по числу
фигур на доске** (`k = min(max(фигур − 2, 0) / 4, 7)`, фигуры считаются с
королями): корзина выбирается при оценке, аккумулятор от неё не зависит, так
что цена `Update` не меняется. Формат весов **DYNN v2** — заголовок с полем
`Buckets` (v1, одна корзина, читается как раньше); любая ошибка заголовка,
размера или CRC отклоняет файл. Лучшая сеть на 2026-10-13 — `net-g5l1.bin`
(H = 256, 8 корзин, ~52 млн позиций, размечены предыдущими сетями на глубине
9–10). Подгружается опцией UCI `EvalFile`; без неё работает ручной эвал.

**Цель обучения** — `λ·sigmoid(score/K) + (1−λ)·исход партии` со стороны хода,
`K = 400`. Лучшая сеть обучена с **`λ = 1`**: метка — только оценка поиска
глубины 9–10, результат партии не учитывается (при `λ = 0,75` он был шумом:
`λ = 1` дал +32 Elo при тех же данных и архитектуре). Масштаб `K = 300` и `500`
эффекта не дали. Корзины (+47 Elo) и `λ = 1` — два главных шага после смены
поколений данных; H = 512 в трёх попытках значимого выигрыша не показал и
вдвое медленнее.

### Ленивая генерация ходов в `negamax`

Раньше `negamax` в самом начале строил полный список легальных ходов ради
проверки мата/пата/ничьей, хотя узлы, выходящие по reverse futility,
razoring или на `depth == 0`, сам список не использовали. Теперь список
строится только перед null-move и циклом ходов (`ensureMoves`), а ранние
выходы спрашивают лишь «есть ли хоть один легальный ход»
(`movegen.HasLegalMove`: тот же генератор `generateLegal`, но с выходом
после первого хода; решение о легальности принимается в одном месте).
Замыкание `terminal()` вызывается перед каждым выходом и побочным эффектом,
которым раньше предшествовала проверка статуса, поэтому поведение не
меняется: оценки и число узлов совпали до единицы на 12 позициях
(мат, пат, пешечные эндшпили, миттельшпиль) и на трёх бенчмарках
(8 225 874 узла). Время до фиксированной глубины 4963 → 4533 мс, ≈ −8,7%.
Тест `TestHasLegalMoveMatchesFullGeneration` сверяет `HasLegalMove` и
`GameStatusFromAny` с полной генерацией на 300 случайных партиях.

### Quiescence: взятия без предварительной проверки легальности

Not-in-check путь `quiescenceBody` берёт `movegen.GeneratePseudoLegalCaptures`
(без фильтра легальности), а `quiescenceLoop` отбрасывает незаконное взятие
сразу после уже нужного `MakeMove` (`IsSquareAttacked` на своём короле).
Раньше `GenerateLegalCaptures` делал make/unmake каждого взятия ради проверки
(≈ 7,5% профиля), и затем цикл делал тот же `MakeMove` второй раз. Путь в
шахе по-прежнему использует `GenerateLegalMoves`. Узлы идентичны до
единицы (8 225 874 на трёх позициях), время до фиксированной глубины
5092 → 4858 мс, ≈ −4,6% (три попеременных прогона). Матч не нужен:
поведение не меняется.

### TT в quiescence

`quiescenceSearch` читает и пишет ту же TT, что `negamax`. Чтение — по
тем же правилам границ (точное значение, нижняя/верхняя граница против
окна), мат-счёт поправляется по `ply`. Запись — с глубиной 0, не
затирает более глубокую запись той же позиции и не пишется, если поиск
прерван по времени (`tt.aborted()`). Записи quiescence не несут хода,
поэтому в `negamax` признак «есть ход из TT» теперь требует ненулевого
хода; иначе такие записи молча отключали бы IID. Измерено:
`-openbook=assets/gm2001.bin`, 100 мс, две серии по 300 пар (seed 1–2,
один `-carry`), SPRT 0/10 принял H1 на 382 парах второй серии:
+203 =414 −147, **+25.5 ± 15.9 Elo** (по парам), нижняя граница ≈ +9.6.
Точечная оценка может быть завышена из-за ранней остановки. На 500 мс не
проверено.

### SEE-pruning взятий

`seePruneMaxDepth = 6`, `seePruneMargin = 100`: в цикле ходов
`negamax`, при тех же защитах, что у LMP (не корень, не шах, не у
границ мата), простое взятие (`board.Capture`, не en passant и не
превращение) не первым по порядку пропускается без поиска, если
`SEE(b, m) < -seePruneMargin*depth`. Измерено: +17.7 ± 15.1 Elo на 1200
партиях (две серии по разным дебютам), LLR ≈ 2.15 — SPRT-граница не
достигнута, но интервал не включает 0.

### LMR по логарифму

Редукция первой пробы — `lmrReductionFor(depth, i)` из
`lmrTable[depth][i] = int(0.75 + ln(depth)*ln(i)/2.25)`, не меньше 1 и
не глубже `depth-2`. Измерено: +14.8 ± 15.3 Elo на 1200 партиях (две
серии по разным дебютам), SPRT формально не закрылся (LLR ≈ 1.6),
принято как стандартная техника с устойчиво положительным
результатом.

### LMR/PVS, aspiration windows

Без изменений по сути. LMR встроен внутрь PVS, не отдельной веткой:
первый ход — сразу полное окно; каждый следующий — сначала дешёвая
проба нулевым окном, для поздних тихих ходов (после
`lmrFullDepthMoves`-го, на глубине `>= lmrMinDepth`, не взятие/
превращение/шах, не под шахом) первая проба на уменьшенной глубине.
Aspiration windows — вокруг счёта предыдущей итерации итеративного
углубления, полный пересчёт при промахе окна.

### SEE (`see.go`)

`SEE` — тонкая обёртка над `seeFull`: если у противника нет атакующих на
`to` (с учётом вскрытой линии за ушедшей фигурой; взятие на проходе всегда
идёт в `seeFull`), ответ — стоимость жертвы. `seeFull` строит `seeBoard`
копией массива битбордов (`Board.PieceBitboards()`), а не двенадцатью
вызовами `Pieces()`. Результат тот же, что у полного алгоритма
(`TestSEEFastPathMatchesFull`); время до глубины ≈ −4,5%.

Без изменений. На битбордах, не копия `[64]Piece` — `seeBoard`,
собственный лёгкий мутируемый снимок. Используется в
`quiescenceSearch` (отсечение взятий с `SEE < 0`, сортировка
оставшихся по SEE); в ordering'е `negamax` по всему дереву НЕ
используется — там MVV-LVA.

### `quiescenceSearch` + delta pruning

Под шахом — полный `GenerateLegalMoves`; не под шахом (большинство
узлов) — `GenerateLegalCaptures`. **Delta pruning** — дешёвый фильтр
ПЕРЕД полноценным SEE: `deltaMargin = 200`, взятие отбрасывается без
вызова `SEE`, если даже `standPat + стоимость взятой фигуры +
deltaMargin < alpha` — SEE в лучшем случае может только подтвердить ту
же самую сумму материала, никогда не изобрести лишний, так что если
даже оптимистичная оценка не дотягивает, звать SEE незачем. Никогда не
режет en passant (`PieceAt(m.To)` там в принципе смотрит не туда) и
promotion-capture (голая стоимость взятой фигуры занижает реальный
выигрыш — не считает новую фигуру).

### Lazy SMP (`BestMoveTimedMT`/`BestMoveTimedMTShared`)

Несколько горутин независимо гоняют `negamax` на одной позиции, деля
только `*sharedTT` — общая хеш-таблица ускоряет отдельные ветки,
остальное (`killers`/`history`/`searchPath`/`nodes`) у каждого потока
своё через `NewSearch()`. Критично: каждый поток работает со СВОЕЙ
копией доски (`b.Copy()`) — `negamax` мутирует доску на месте через
`MakeMove`/`UnmakeMove`, так что шаринг одного `*board.Board` между
горутинами испортил бы состояние, а не просто гонялся бы по данным.

Выбор итогового хода между потоками: глубже завершённый поиск
побеждает; при равенстве глубины — выше `score` (все потоки искали
одну и ту же сторону на ходу, сравнение честное); при полном равенстве
— меньший номер потока (детерминированность). `Nodes` в итоговом
`SearchInfo` — сумма по всем потокам.

`BestMoveTimedMT` всегда стартует со свежим `*sharedTT` (одноразовый
поиск); `BestMoveTimedMTShared` принимает готовый `*TranspositionTable`
и делит его `.shared` между потоками — это и есть основа межходовой
персистентности ниже. `threads <= 1` в обеих функциях — no-op fallback
на обычный однопоточный путь, ни на что не влияет.

### TT между ходами (UCI-сессия, CLI `-play`)

Реальный пробел, который тоже был закрыт: раньше каждый `go`
(UCI)/каждый ход (`-play`) стартовал с `NewTranspositionTable()` с
нуля — движок никогда не пользовался тем, что уже насчитал на
предыдущих ходах партии (в том числе тем, что противник, скорее всего,
сыграл ход, который сам же движок анализировал на своём предыдущем
ходу). Теперь `internal/uci`'s `session` и CLI `playInteractive`
держат один `*TranspositionTable` на всю партию, и каждый поиск берёт
`session.tt.NewSearch()` (или прокидывает `session.tt` целиком в
`BestMoveTimedMTShared`) вместо холодного старта. Сбрасывается только
по `ucinewgame` (UCI) — не по каждому `position`, поскольку это просто
способ GUI пересказать состояние той же партии, а не сигнал новой.

### `SearchInfo`/`...Info`/`...TT`/`...MT`/`...Shared`-варианты

`BestMoveInfo`/`BestMoveTimedInfo` — как раньше, с колбэком `onInfo` на
каждую завершённую глубину. `...TT`-варианты (`BestMoveInfoTT`,
`BestMoveTimedInfoTT`) принимают готовый `*TranspositionTable` вместо
создания нового — на них построена межходовая персистентность.
`BestMoveTimedMT`/`BestMoveTimedMTShared` — Lazy SMP, `onInfo`
вызывается один раз в конце с агрегированным результатом (не на каждую
глубину каждого потока — это гонялось бы конкурентно и перемешивало
глубины разных потоков).

### Книга дебютов (`opening_book.go`)

Логика без изменений — книга как приоритет перед поиском. Добавлены
`*MT`/`*Shared`-обёртки (`BestMoveTimedWithBookMT`,
`BestMoveTimedWithBookMTShared`, `BestMoveWithBookInfoTT`) поверх
Lazy SMP и межходовой TT — книжный хит по-прежнему никогда не трогает
`tt` вообще, поскольку поиск в этом случае просто не запускается.

## `internal/book`, `cmd/wasm`, `web/`

Без изменений — см. сами файлы, эта работа их не трогала.

## `internal/uci`, `cmd/cli`

`internal/uci`'s `session` и CLI держат персистентный
`*search.TranspositionTable` на всю партию (см. «TT между ходами»
выше) и число потоков (`session.threads`/`-threads`) для
movetime-поиска — Lazy SMP имеет смысл только там, где есть временной
бюджет, чтобы использовать освободившиеся ядра ради БОЛЬШЕЙ глубины;
у фиксированной по глубине выдачи (`go depth N` без `movetime`,
`-play` без `-movetime`) такого бюджета нет, так что `-threads`/
`Threads` там ни на что не влияет.

**Часы (`go wtime/btime/winc/binc/movestogo`).** Без `movetime`, но с
временем стороны на ходу бюджет хода — `clockBudgetMs`: `time/movestogo`
(по умолчанию 30) + `3/4·inc`, не больше трети остатка часов, минус 30 мс
на задержки GUI/сети, не меньше 1 мс. Потолок глубины при этом 64
(`maxClockDepth`), если `depth` не задан явно, — поиск заканчивает бюджет,
а не сессионная глубина. `infinite`/`stop`/`ponder` не реализованы. На силу
в матчах фиксированного времени не влияет; проверено только тестами
(`TestClockBudgetMs`, `TestGoWithClockUsesBudget`), не игрой на сервере.

UCI дополнительно поддерживает `setoption name Threads value N`
(перекрывает стартовое значение на лету) и объявляет его в ответе на
`uci` строкой `option name Threads type spin default N min 1 max 256`.
Опция `Hash` (МБ, 1–4096, по умолчанию 16) создаёт новую таблицу
транспозиций нужного размера (`search.NewTranspositionTableMB`; прежняя
теряется), `ucinewgame` создаёт таблицу текущего размера. Опция `EvalFile`
(строка, по умолчанию `<empty>`) загружает файл весов NNUE (`nnue.Load`,
путь может содержать пробелы) и включает его в поиске; `<empty>` возвращает
ручной эвал; смена эвала создаёт новую хеш-таблицу (старая хранит оценки
другого эвала); файл, который не читается, — `info string` с причиной, сеть
не меняется. У `cmd/cli` то же самое — флаг `-evalfile`. Кроме того,
объявляются шестнадцать параметров поиска из `search.Tunables()`.
`stop`, `infinite` и `ponder` не реализованы.

## `cmd/profile`

```
go run ./cmd/profile [-threads=N] [-budget=5s] [-fen="..."] [-out=cpu.prof] [-evalfile=net.bin]
```

Печатает и узлы в секунду (до последней завершённой глубины); `-evalfile` —
поиск с сетью NNUE (для честного замера собирать с `GOEXPERIMENT=simd`, см.
`NNUE.md`).

Гоняет `search.BestMoveTimedMT` (при `-threads=1` — обычный
однопоточный путь) на фиксированном ВРЕМЕНИ (не глубине — так
сравнение `threads=1` vs `threads=N` остаётся честным при любом
ускорении самого поиска), печатает узлы/счёт по каждой завершённой
глубине и пишет `cpu.prof`. По умолчанию — стартовая позиция;
`-fen="..."` — любая другая, полезно для техник, которым нечего делать
в тихой сбалансированной дебютной позиции (например, для проверки
Late Move Pruning/IID на реальной тактике вместо старта). Постоянный
инструмент проекта, не одноразовый скрипт — использовался на каждом
шаге этой работы для проверки, что очередная правка действительно
что-то изменила, а не просто выглядит правильной на бумаге. Дедлайн
проверяется и внутри дерева (см. «Мгновенная остановка по времени»
выше), так что прогон укладывается в `-budget` с точностью до
нескольких миллисекунд.

## `cmd/match` — матчи между двумя бинарниками

```
go build -o baseline.exe ./cmd/cli      # ДО правки — точка отсчёта
# ... правка ...
go build -o candidate.exe ./cmd/cli
go build -o match.exe ./cmd/match
.\match.exe -a candidate.exe -b baseline.exe -movetime=100 -pairs=100 -concurrency=8
```

Единственный инструмент проекта, который меряет **силу игры**, а не
скорость: играет два UCI-бинарника (`cmd/cli -uci -book=`) парами —
один дебют, цвета меняются. Печатает W/D/L, Elo-разницу с 95%
доверительным интервалом; с `-elo0=0 -elo1=5` — ещё и SPRT
(нормальное приближение, alpha=beta=0.05), останавливаясь, как только
пересечена граница. Ранняя остановка по SPRT завышает точечную оценку —
крупный результат на малом числе пар перепроверять серией фиксированной
длины без `-elo0/-elo1`.

**Чужие UCI-движки.** `-a`/`-b` принимают любой UCI-бинарник.
`-aargs`/`-bargs` задают аргументы командной строки движка: по умолчанию
`cli` (флаги `cmd/cli`: `-uci -book= -depth=64 -threads=N`), любое другое
значение режется по пробелам и передаётся как есть, пустое — без аргументов
(`-bargs=`, нужно для Stockfish). Опции движка — `-aopts`/`-bopts`
(`Имя=значение,...`). Гаунтлет против Stockfish 19 из `engines/sf19/`
(скачан с официального репозитория, релиз sf_19):

```
.\match.exe -a baseline.exe -b engines\sf19\stockfish\stockfish-windows-x86-64-universal.exe -bargs= `
  -bopts "Threads=1,Hash=16,UCI_LimitStrength=true,UCI_Elo=2650" `
  -movetime=500 -pairs=120 -concurrency=8 -openbook=assets/gm2001.bin -seed=1 "-carry=carry-sf2650.json"
```

`UCI_Elo` у Stockfish — от 1320 до 3190; результаты и оговорки по калибровке
на коротком контроле — в `plan.md`, пункт 1.

**Статистика — по парам (`penta`).** Результат пары (две партии одного
дебюта с разными цветами) — одно из пяти значений 0/0.5/1/1.5/2 очка
для A; дисперсия считается по счёту пары, а не по счёту партии.
Партии одной пары скоррелированы (перекос дебюта в одну сторону гасит
себя внутри пары), и поэтому интервал по парам уже, чем «по партиям»,
которое эту корреляцию игнорировало. Пара засчитывается, когда доиграны
обе партии (параллельные партии пары кончаются в произвольном порядке;
первая ждёт вторую в `pending`). Итоговая строка печатает и старую
оценку по партиям — для сравнения. W/D/L остаются как сводка.
Формулы: `eloReportFrom`/`llrFrom` общие для `tally` и `penta`.

**Adjudication** (`-adjwin`, `-adjwinmoves`, `-adjdraw`, `-adjdrawmoves`,
`-adjdrawply`): партия обрывается по оценкам самих движков (последняя
`score cp/mate` перед `bestmove`, пересчитанная на точку зрения белых).
Победа — если последние `2*adjwinmoves` полуходов оба движка показывали
не меньше `adjwin` сантипешек (по умолчанию 1000 и 3 хода) в пользу одной
стороны; полуход без счёта обрывает серию. Ничья (по умолчанию выключена,
`-adjdraw=0`) — если с `adjdrawply`-го полухода оценки держались в
±`adjdraw`. Причина партии в журнале: `adjudicated win/draw`. Включение
меняет исход редких партий, поэтому результаты с adjudication и без него
в один `-carry` лучше не смешивать.

**`-carry=файл`** копит результаты между запусками (JSON: счёт пар,
W/D/L, использованные seed'ы): при старте читается, Elo/SPRT идут по
всем парам сразу, в конце файл перезаписывается. Для каждой серии нужен
свой `-seed` (при совпадении с уже записанным — предупреждение:
дебюты повторятся, и серии нельзя честно складывать). Недоигранные пары
(остановка по SPRT посередине) в файл не попадают. Дебюты (код — `internal/openings`, общий с `cmd/gendata`) — из `-openings` (FEN на строку), из `-openbook` (случайный
обход Polyglot-книги на `-openplies` полуходов с учётом весов; не
вышедшие из книги или несбалансированные ветки отбрасываются) или
генерируются по `-seed`: 8 случайных ходов без взятий; в обоих
сгенерированных вариантах отбор по `|eval| <= 80` и без дублей (один
seed = одни и те же дебюты для обеих версий). Книга самих движков в матче
отключена (`-book=`), иначе обе стороны разыгрывали бы одну теорию.
Игрок, сломавшийся/зависший/сыгравший нелегальный ход, проигрывает.
Калибровка: копия против копии на 160 партиях дала около нуля (+52 и
−48 при разной раскладке ролей, каждый ±62) — перекоса в инструменте
нет. 80 партий по 100 мс на 8 параллельных играх — около 1м40с.
Параллельные партии делят CPU: держите `concurrency*2*threads` не выше
числа физических ядер.

### Мгновенная остановка по времени

`TranspositionTable.shouldStop()` читает часы раз в 2048 узлов из
`negamax`/`quiescenceSearch`; сработав, поиск разворачивается, ничего не
записывая в TT (счета оборванных узлов — мусор), а
`runIterativeDeepening` отбрасывает недоделанную итерацию целиком и
возвращает ход последней завершённой глубины. Глубина 1 всегда
доигрывается (иначе нечего вернуть). Раньше дедлайн проверялся только
между глубинами, и одна глубина в позиции с матом могла занять 38M
узлов при бюджете 50 мс — это делало матчи бессмысленными.