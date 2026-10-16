import Segmented from "../ui/Segmented.jsx";
import { Row, SaveBar, SoonRow, Switch } from "./controls.jsx";

const CASTLING = [
  ["standard", "Стандартно"],
  ["rook", "Король на ладью"],
  ["both", "Оба способа"],
];
const CASTLING_HINT = {
  standard: "Рокировка: нажмите на короля и на поле, куда он идёт (на две клетки), или перенесите короля на это поле. Подходит тем, кто хочет избежать мискликов.",
  rook: "Рокировка: нажмите на короля и на свою ладью или перенесите короля на ладью. Ход короля на две клетки рокировкой не считается.",
  both: "Рокировку можно сделать обоими способами: королём на две клетки или королём на ладью.",
};
const LEVELS = [
  ["random", "Случайно"],
  ["easier", "Легче"],
  ["normal", "По вашему уровню"],
  ["harder", "Сложнее"],
];
const LEVEL_HINT = {
  random: "Сложность каждой задачи случайная, в пределах 300 пунктов от вашего рейтинга.",
  easier: "Задачи чуть легче вашего рейтинга (на 150 пунктов).",
  normal: "Все задачи подобраны под ваш уровень.",
  harder: "Задачи чуть сложнее вашего рейтинга (на 150 пунктов): рейтинг растёт быстрее, но и ошибаться придётся чаще.",
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
  return (
    <>
      <h2 className="settings__h">Игровой процесс</h2>
      <p className="settings__sub">Управляйте тем, как проходят партии, вызовы, задачи и игра с ботами.</p>

      <h3 className="settings__group">Общие</h3>
      <Row
        title="Всегда превращать в ферзя"
        hint="Пешка, дошедшая до последней горизонтали, сразу становится ферзём. Чтобы выбрать другую фигуру, удерживайте клавишу Alt при ходе."
        control={sw(draft, set, "alwaysQueen", "Всегда превращать в ферзя")}
      />
      <Row
        title="Подтверждать сдачу или ничью"
        hint="Перед тем как сдаться или предложить ничью, сайт спросит ещё раз."
        control={sw(draft, set, "confirmGiveUp", "Подтверждать сдачу или ничью")}
      />
      <Row
        title="Показывать допустимые ходы"
        hint="Точки и кольца на полях, куда может пойти выбранная фигура."
        control={sw(draft, set, "showLegalMoves", "Показывать допустимые ходы")}
      />
      <Row
        title="Всегда использовать режим «Доска в центре»"
        hint="На страницах с доской остаётся только доска, она становится крупной. Включать и выключать режим можно и значком в правом нижнем углу (выход: Esc)."
        control={sw(draft, set, "focusBoard", "Всегда использовать режим «Доска в центре»")}
      />
      <Row
        title="Белые всегда внизу доски"
        hint="Доска не переворачивается, когда вы играете чёрными."
        control={sw(draft, set, "whiteBottom", "Белые всегда внизу доски")}
      />
      <Row title="Способ рокировки" hint={CASTLING_HINT[draft.castling]}>
        <Segmented label="Способ рокировки" value={draft.castling} onChange={(v) => set({ castling: v })} options={CASTLING} />
      </Row>
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">Онлайн</h3>
      <Row
        title="Включить премувы"
        hint="Сделайте ход заранее, пока ходит соперник: он сыграется сам, как только станет возможным. Можно поставить несколько премувов подряд (до 30: в эндшпиле на времени такие серии решают), каждый следующий выбирается на доске после предыдущих. Если очередной ход окажется невозможным, очередь сбрасывается. Отмена всех: Esc или правая кнопка мыши."
        control={sw(draft, set, "premoves", "Включить премувы")}
      />
      <Row
        title="Предупреждение об окончании времени"
        hint="Часы краснеют, а за 10 секунд до конца звучит сигнал."
        control={sw(draft, set, "timeWarning", "Предупреждение об окончании времени")}
      />
      <Row
        title="Подтверждать ход"
        hint="Ход отправляется соперникам после нажатия «Подтвердить». Можно отменить и сходить иначе."
        control={sw(draft, set, "confirmMove", "Подтверждать ход")}
      />
      <Row
        title="Разрешить несколько партий одновременно"
        hint="Если выключено, пока идёт одна партия (с человеком или ботом), начать вторую нельзя."
        control={sw(draft, set, "multiGames", "Разрешить несколько партий одновременно")}
      />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">Заочные партии</h3>
      <SoonRow title="Подтверждать ход" hint="Подтверждать сделанный ход." />
      <SoonRow title="Автоматически заявлять о победе" hint="Если у соперника закончится время, победа засчитывается без вашего подтверждения." />
      <SoonRow title="Режим отпуска" hint="Приостановить ваши партии." />
      <SoonRow title="После хода" hint="Перейти к следующей доступной партии." />
      <SoonRow title="Дней на ход" hint="Количество дней на ход в заочных партиях." />

      <h3 className="settings__group">Вызовы</h3>
      <Row
        title="Рейтинг открытого вызова"
        hint="Насколько рейтинг соперника, которого подбирает сайт, может отличаться от вашего. Пустое поле значит без ограничений: ∞. Соперник тоже должен подходить под свои границы."
      >
        <div className="limits">
          <span className="limits__label">Не слабее чем на</span>
          <Limit value={draft.seekDown} onChange={(v) => set({ seekDown: v })} label="Насколько слабее можно" sign="−" />
          <span className="limits__label">и не сильнее чем на</span>
          <Limit value={draft.seekUp} onChange={(v) => set({ seekUp: v })} label="Насколько сильнее можно" sign="+" />
        </div>
      </Row>
      <SoonRow title="Разрешить вызовы" hint="Все пользователи, только друзья или никто. Для этого нужны друзья и личные вызовы." />
      <SoonRow title="Входящий рейтинг" hint="Диапазон рейтинга шахматистов, которые могут отправлять вам вызовы." />
      <SoonRow title="Игнорировать рейтинг при входящих вызовах от друзей" />
      <SoonRow title="Вызовы оффлайн" hint="Вызовы, когда вас нет на сайте, и уведомления о них." />
      <SoonRow title="Уведомления о вызовах" hint="Показывать уведомления при получении вызова: от всех или только от друзей." />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">Анализ</h3>
      <Row
        title="Оценка движка"
        hint="Показывать оценку движка (шкала и число) на странице разбора партии."
        control={sw(draft, set, "showEval", "Оценка движка")}
      />
      <Row
        title="Показывать разбор после игры"
        hint="Когда партия закончилась, сайт сам запускает разбор движком и показывает его под доской."
        control={sw(draft, set, "autoReview", "Показывать разбор после игры")}
      />
      <SoonRow title="Показывать время" hint="Время, потраченное на каждый ход. Сайт пока не хранит время хода." />
      <Row
        title="Показывать иконки классификации ходов в списке ходов"
        hint="Блестящие ходы, зевки и другие значки рядом с ходами в разборе."
        control={sw(draft, set, "moveIcons", "Показывать иконки классификации ходов")}
      />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

      <h3 className="settings__group">Задачи</h3>
      <Row title="Уровень сложности" hint={LEVEL_HINT[draft.puzzleLevel]}>
        <Segmented label="Уровень сложности" value={draft.puzzleLevel} onChange={(v) => set({ puzzleLevel: v })} options={LEVELS} />
      </Row>
      <SoonRow title="Собирать очки за решение задач" hint="Режим очков вместо рейтинга. Сейчас задачи решаются только на рейтинг." />
      <Row
        title="Всегда показывать рейтинг"
        hint="Рейтинг в задачах виден на странице. Если выключить, он скрыт, но продолжает считаться."
        control={sw(draft, set, "puzzleRating", "Всегда показывать рейтинг")}
      />
      <Row
        title="Показать темы задачи"
        hint="Темы (мотивы) задачи видны сразу, а не только после решения."
        control={sw(draft, set, "puzzleThemes", "Показать темы задачи")}
      />
      <SoonRow title="Показать комментарии к ошибкам" hint="Тренер поможет объяснить ваши ошибки." />
      <SoonRow title="Показать подсказки в чате" hint="Помощь тренера, когда вы просите совет." />
      <Row
        title="Показывать таймер"
        hint="Часы, сколько времени вы решаете задачу."
        control={sw(draft, set, "puzzleTimer", "Показывать таймер")}
      />
      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
    </>
  );
}
