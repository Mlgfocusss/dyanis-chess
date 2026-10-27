import Icon from "../ui/Icon.jsx";
import { useCommunity } from "./CommunityContext.jsx";
import { useT } from "../i18n/index.js";

// Small notices in the corner: a challenge came, a challenge was accepted.
export default function Notices() {
  const t = useT();
  const { notices, dismiss } = useCommunity();
  if (!notices.length) return null;
  return (
    <div className="notices" role="region" aria-label={t("notices.1")} aria-live="polite">
      {notices.map((n) => (
        <div key={n.id} className="notice">
          <Icon name="bell" size={18} />
          <span className="notice__text">{n.text}</span>
          {n.href && (
            <a className="btn btn--primary notice__go" href={n.href} onClick={() => dismiss(n.id)}>
              {n.action || t("notices.2")}
            </a>
          )}
          <button type="button" className="notice__x" aria-label={t("notices.3")} onClick={() => dismiss(n.id)}>
            <Icon name="close" size={16} />
          </button>
        </div>
      ))}
    </div>
  );
}
