import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { DELETE_WORD } from "../auth/api.js";
import Avatar from "../auth/Avatar.jsx";
import { GoogleUpgrade, SignOut, formatDate } from "../auth/AccountParts.jsx";
import { Row } from "./controls.jsx";
import SignInOffer from "./SignInOffer.jsx";
import { confirmsDelete } from "./nameRules.js";

// "Учётная запись": what kind of account this is, signing out, deleting it for good.
export default function AccountPanel() {
  const { status, user } = useAuth();
  const [deleted, setDeleted] = useState(false);

  if (status !== "ready" || !user) {
    return (
      <>
        <h2 className="settings__h">Учётная запись</h2>
        {deleted && (
          <div className="savebar__ok settings__done" role="status">
            Аккаунт удалён вместе с вашими данными.
          </div>
        )}
        <SignInOffer />
      </>
    );
  }

  const isGuest = user.kind === "guest";
  const since = formatDate(user.createdAt);
  return (
    <>
      <h2 className="settings__h">Учётная запись</h2>
      <p className="settings__sub">Как вы входите на сайт и что будет с вашими данными.</p>

      <div className="pcard">
        <Avatar user={user} size={64} />
        <div className="pcard__who">
          <div className="pcard__name">{user.name}</div>
          <div className="pcard__meta">
            <a href="#settings/profile">Изменить имя</a>
          </div>
        </div>
      </div>

      <dl className="facts">
        <div className="facts__row">
          <dt>Вид аккаунта</dt>
          <dd>{isGuest ? "Гость" : "Google"}</dd>
        </div>
        {!isGuest && user.email && (
          <div className="facts__row">
            <dt>Почта</dt>
            <dd>{user.email}</dd>
          </div>
        )}
        {since && (
          <div className="facts__row">
            <dt>Дата регистрации</dt>
            <dd>{since}</dd>
          </div>
        )}
      </dl>

      {isGuest && (
        <div className="settings__upgrade">
          <GoogleUpgrade />
        </div>
      )}

      <Row title="Выйти" hint={isGuest ? "Гостя после выхода не вернуть: привяжите Google, если хотите сохранить партии." : "Закончить сеанс на этом устройстве. Партии и настройки останутся в аккаунте."}>
        <SignOut icon />
      </Row>

      <Row
        title="Удалить аккаунт"
        hint="Удаляются сам аккаунт, настройки, рейтинги, прогресс в задачах и против ботов, репертуар дебютов. Разборы партий остаются без имени, партии с другими игроками остаются у соперника. Отменить это нельзя."
      >
        <DeleteAccount user={user} onDone={() => setDeleted(true)} />
      </Row>
    </>
  );
}

function DeleteAccount({ user, onDone }) {
  const { deleteAccount } = useAuth();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const ok = confirmsDelete(typed, DELETE_WORD, user.name);

  function close() {
    setOpen(false);
    setTyped("");
    setError("");
  }

  async function onSubmit(e) {
    e.preventDefault();
    if (!ok || busy) return;
    setBusy(true);
    setError("");
    try {
      await deleteAccount();
      onDone();
    } catch (err) {
      setError(err.message);
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <button type="button" className="btn btn--danger" onClick={() => setOpen(true)}>
        Удалить аккаунт…
      </button>
    );
  }
  return (
    <form className="delform" onSubmit={onSubmit}>
      <label className="delform__label" htmlFor="delete-confirm">
        Чтобы подтвердить, введите слово <b>{DELETE_WORD}</b> или своё имя <b>{user.name}</b>.
      </label>
      <input
        id="delete-confirm"
        className="field delform__input"
        value={typed}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => setTyped(e.target.value)}
      />
      {error && (
        <p className="nameform__error" role="alert">
          {error}
        </p>
      )}
      <div className="savebar">
        <button type="button" className="btn btn--ghost" onClick={close} disabled={busy}>
          Отмена
        </button>
        <button type="submit" className="btn btn--danger" disabled={!ok || busy}>
          Удалить навсегда
        </button>
      </div>
    </form>
  );
}
