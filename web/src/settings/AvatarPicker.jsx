import Avatar from "../auth/Avatar.jsx";
import { AVATAR_PRESETS, presetValue } from "../auth/avatarPresets.js";
import Icon from "../ui/Icon.jsx";

// The grid of avatars to choose from: the letter of the name (the default), the Google photo for a person
// who signed in with Google, and the twelve ready-made ones. choice is "", "google" or "preset:<id>".
export default function AvatarPicker({ choice, onChange, googleAvatarUrl, name }) {
  const items = [
    { value: "", label: "Буква", user: { name } },
    ...(googleAvatarUrl ? [{ value: "google", label: "Фото Google", user: { name, avatarUrl: googleAvatarUrl } }] : []),
    ...AVATAR_PRESETS.map((p) => ({ value: presetValue(p.id), label: p.label, user: { name, avatarUrl: presetValue(p.id) } })),
  ];
  return (
    <div className="apick" role="group" aria-label="Аватар">
      {items.map((it) => {
        const on = it.value === choice;
        return (
          <button
            key={it.value || "letter"}
            type="button"
            className={"thumb apick__item" + (on ? " thumb--selected" : "")}
            aria-pressed={on}
            onClick={() => onChange(it.value)}
          >
            <span className="thumb__art">
              <Avatar user={it.user} size={48} />
            </span>
            <span className="thumb__name">{it.label}</span>
            {on && (
              <span className="thumb__check" aria-hidden="true">
                <Icon name="check" size={14} />
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
