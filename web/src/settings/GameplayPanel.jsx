import Segmented from "../ui/Segmented.jsx";
import VacationRow from "../community/VacationRow.jsx";
import NotifyRows from "../notify/NotifyRows.jsx";
import { CORR_DAYS, daysText } from "../community/logic.js";
import { Row, SaveBar, SoonRow, Switch } from "./controls.jsx";
import { useT, tOpts } from "../i18n/index.js";

const CASTLING = [
  ["standard", "gameplay.castlingStandard"],
  ["rook", "gameplay.castlingRook"],
  ["both", "gameplay.castlingBoth"],
];
const CASTLING_HINT = {
  standard: "gameplay.castlingStandardHint",
  rook: "gameplay.castlingRookHint",
  both: "gameplay.castlingBothHint",
};
const LEVELS = [
  ["random", "bots.random"],
  ["easier", "gameplayPz.easier"],
  ["normal", "gameplayPz.normal"],
  ["harder", "gameplayPz.harder"],
];
const LEVEL_HINT = {
  random: "gameplayPz.randomHint",
  easier: "gameplayPz.easierHint",
  normal: "gameplayPz.normalHint",
  harder: "gameplayPz.harderHint",
};

const FROM = [
  ["all", "gameplay.fromAll"],
  ["friends", "gameplay.fromFriends"],
  ["none", "gameplay.fromNone"],
];
const FROM_HINT = {
  all: "gameplay.fromAllHint",
  friends: "gameplay.fromFriendsHint",
  none: "gameplay.fromNoneHint",
};
const NOTIFY = [
  ["all", "gameplay.notifyAll"],
  ["friends", "gameplay.notifyFriends"],
  ["none", "gameplay.notifyNone"],
];
const NOTIFY_HINT = {
  all: "gameplay.notifyAllHint",
  friends: "gameplay.notifyFriendsHint",
  none: "gameplay.notifyNoneHint",
};

function sw(draft, set, key, label) {
  return <Switch checked={draft[key]} onChange={(v) => set({ [key]: v })} label={label} />;
}

// A rating difference (points) that "no limit" is written as an empty field for.
function Limit({ value, onChange, label, sign }) {
  return (
    <label className="limit">
      <span className="limit__sign" aria-hidden="true">
        {sign}
      </span>
      <input
        className="field limit__input"
        type="number"
        min="0"
        max="3000"
        step="50"
        inputMode="numeric"
        placeholder="∞"
        value={value || ""}
        aria-label={label}
        onChange={(e) => onChange(e.target.value === "" ? 0 : Math.max(0, Math.min(3000, Math.round(Number(e.target.value)) || 0)))}
      />
    </label>
  );
}

// "Игровой процесс": how a game is played. Rows marked "скоро" wait for features the site does not have yet.
export default function GameplayPanel({ draft, set, dirty, justSaved, onSave, onCancel }) {
  const t = useT();
  return (
    <>
      <h2 className="settings__h">{t("gameplay.title")}</h2>
      <p className="settings__sub">{t("gameplay.28")}</p>

      <h3 className="settings__group">{t("gameplay.29")}</h3>
      <Row
        title={t("gameplay.30")}
        hint={t("gameplay.31")}
        control={sw(draft, set, "alwaysQueen", t("gameplay.30"))}
      />
      <Row
        title={t("gameplay.32")}
        hint={t("gameplay.33")}
        control={sw(draft, set, "confirmGiveUp", t("gameplay.32"))}
      />
      <Row
        title={t("gameplay.34")}
        hint={t("gameplay.35")}
        control={sw(draft, set, "showLegalMoves", t("gameplay.34"))}
      />
      <Row
        title={t("gameplay.36")}
        hint={t("gameplay.37")}
        control={sw(draft, set, "focusBoard", t("gameplay.36"))}
      />
      <Row
        title={t("gameplay.38")}
        hint={t("gameplay.39")}
        control={sw(draft, set, "whiteBottom", t("gameplay.38"))}
      />
      <Row title={t("gameplay.40")} hint={t(CASTLING_HINT[draft.castling])}>
        <Segmented label={t("gameplay.40")} value={draft.castling} onChange={(v) => set({ castling: v })} options={CASTLING.map(([v, k]) => [v, t(k)])} />
      </Row>
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">{t("gameplay.41")}</h3>
      <Row
        title={t("gameplay.42")}
        hint={t("gameplay.43")}
        control={sw(draft, set, "premoveOn", t("gameplay.42"))}
      />
      <Row
        title={t("gameplay.44")}
        hint={t("gameplay.45")}
        control={sw(draft, set, "timeWarning", t("gameplay.44"))}
      />
      <Row
        title={t("gameplay.46")}
        hint={t("gameplay.47")}
        control={sw(draft, set, "confirmMove", t("gameplay.46"))}
      />
      <Row
        title={t("gameplay.48")}
        hint={t("gameplay.49")}
        control={sw(draft, set, "multiGames", t("gameplay.48"))}
      />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">{t("gameplay.50")}</h3>
      <Row
        title={t("gameplay.46")}
        hint={t("gameplay.51")}
        control={sw(draft, set, "corrConfirmMove", t("gameplay.52"))}
      />
      <Row
        title={t("gameplay.53")}
        hint={t("gameplay.54")}
        control={sw(draft, set, "corrAutoClaim", t("gameplay.53"))}
      />
      <Row
        title={t("gameplay.55")}
        hint={t("gameplay.56")}
        control={sw(draft, set, "corrNextGame", t("gameplay.55"))}
      />
      <Row title={t("gameplay.57")} hint={t("gameplay.58")}>
        <select className="field" aria-label={t("gameplay.57")} value={draft.corrDays} onChange={(e) => set({ corrDays: Number(e.target.value) })}>
          {CORR_DAYS.map((d) => (
            <option key={d} value={d}>
              {daysText(d)}
            </option>
          ))}
        </select>
      </Row>
      <NotifyRows draft={draft} set={set} />
      <VacationRow />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">{t("gameplay.59")}</h3>
      <Row
        title={t("gameplay.60")}
        hint={t("gameplay.61")}
      >
        <div className="limits">
          <span className="limits__label">{t("gameplay.62")}</span>
          <Limit value={draft.seekDown} onChange={(v) => set({ seekDown: v })} label={t("gameplay.63")} sign="−" />
          <span className="limits__label">{t("gameplay.64")}</span>
          <Limit value={draft.seekUp} onChange={(v) => set({ seekUp: v })} label={t("gameplay.65")} sign="+" />
        </div>
      </Row>
      <Row title={t("gameplay.66")} hint={t(FROM_HINT[draft.challengesFrom])}>
        <Segmented label={t("gameplay.67")} value={draft.challengesFrom} onChange={(v) => set({ challengesFrom: v })} options={FROM.map(([v, k]) => [v, t(k)])} />
      </Row>
      <Row
        title={t("gameplay.68")}
        hint={t("gameplay.69")}
      >
        <div className="limits">
          <span className="limits__label">{t("gameplay.70")}</span>
          <Limit value={draft.challengeMin} onChange={(v) => set({ challengeMin: v })} label={t("gameplay.71")} sign="≥" />
          <span className="limits__label">{t("gameplay.72")}</span>
          <Limit value={draft.challengeMax} onChange={(v) => set({ challengeMax: v })} label={t("gameplay.73")} sign="≤" />
        </div>
      </Row>
      <Row
        title={t("gameplay.74")}
        hint={t("gameplay.75")}
        control={sw(draft, set, "challengeFriendsIgnoreRating", t("gameplay.76"))}
      />
      <Row
        title={t("gameplay.77")}
        hint={t("gameplay.78")}
        control={sw(draft, set, "challengeOffline", t("gameplay.77"))}
      />
      <Row title={t("gameplay.79")} hint={t(NOTIFY_HINT[draft.challengeNotify])}>
        <Segmented label={t("gameplay.79")} value={draft.challengeNotify} onChange={(v) => set({ challengeNotify: v })} options={NOTIFY.map(([v, k]) => [v, t(k)])} />
      </Row>
      <p className="settings__sub">{t("gameplay.80")}</p>
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">{t("gameplay.81")}</h3>
      <Row
        title={t("gameplay.82")}
        hint={t("gameplay.83")}
        control={sw(draft, set, "showEval", t("gameplay.82"))}
      />
      <Row
        title={t("gameplay.84")}
        hint={t("gameplay.85")}
        control={sw(draft, set, "showCoach", t("gameplay.84"))}
      />
      <Row
        title={t("gameplay.86")}
        hint={t("gameplay.87")}
        control={sw(draft, set, "engineArrows", t("gameplay.86"))}
      />
      <Row
        title={t("gameplay.88")}
        hint={t("gameplay.89")}
        control={sw(draft, set, "autoReview", t("gameplay.88"))}
      />
      <Row
        title={t("notify.moveTimeTitle")}
        hint={t("notify.moveTimeHint")}
        control={sw(draft, set, "showMoveTime", t("notify.moveTimeTitle"))}
      />
      <Row
        title={t("gameplay.92")}
        hint={t("gameplay.93")}
        control={sw(draft, set, "moveIcons", t("gameplay.94"))}
      />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">{t("menu.puzzles")}</h3>
      <Row title={t("gameplayPz.10")} hint={t(LEVEL_HINT[draft.puzzleLevel])}>
        <Segmented label={t("gameplayPz.10")} value={draft.puzzleLevel} onChange={(v) => set({ puzzleLevel: v })} options={tOpts(t, LEVELS)} />
      </Row>
      <Row
        title={t("gameplayPz.11")}
        hint={t("gameplayPz.12")}
        control={sw(draft, set, "puzzlePoints", t("gameplayPz.11"))}
      />
      <Row
        title={t("gameplayPz.13")}
        hint={t("gameplayPz.14")}
        control={sw(draft, set, "puzzleRating", t("gameplayPz.13"))}
      />
      <Row
        title={t("gameplayPz.15")}
        hint={t("gameplayPz.16")}
        control={sw(draft, set, "puzzleThemes", t("gameplayPz.15"))}
      />
      <SoonRow title={t("gameplayPz.17")} hint={t("gameplayPz.18")} />
      <SoonRow title={t("gameplayPz.19")} hint={t("gameplayPz.20")} />
      <Row
        title={t("gameplayPz.21")}
        hint={t("gameplayPz.22")}
        control={sw(draft, set, "puzzleTimer", t("gameplayPz.21"))}
      />
      <Row
        title={t("gameplayPz.23")}
        hint={t("gameplayPz.24")}
        control={sw(draft, set, "puzzleAutoNext", t("gameplayPz.23"))}
      />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
    </>
  );
}
