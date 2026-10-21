import { useEffect, useId, useMemo, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { scrollWithin } from "../ui/scroll.js";
import { countryName, searchCountries } from "./countryList.js";
import "./countries.css";

// A field to choose a country: type to search the list by name (Russian or English) or code, or open it
// and scroll. The first line clears the choice (emptyLabel). Keyboard: the arrows move, Enter chooses,
// Esc closes. It is the ARIA combobox with a list: the focus stays in the text field.
export default function CountryPicker({ id, value, onChange, countries, emptyLabel = "Не указана", disabled = false, ariaLabel }) {
  const auto = useId();
  const inputId = id || `cpick-${auto}`;
  const listId = `${inputId}-list`;
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const listRef = useRef(null);

  // The lines shown: the "none" line first (only while nothing is typed), then the matches.
  const options = useMemo(() => {
    const found = searchCountries(countries, query);
    return query.trim() ? found : [{ code: "", ru: emptyLabel, en: "" }, ...found];
  }, [countries, query, emptyLabel]);

  const selected = value ? countryName(countries, value) : emptyLabel;

  function show() {
    if (open || disabled) return;
    setQuery("");
    const i = value ? countries.findIndex((c) => c.code === value) + 1 : 0;
    setActive(Math.max(i, 0));
    setOpen(true);
  }

  function close() {
    setOpen(false);
    setQuery("");
  }

  function choose(option) {
    if (!option) return;
    onChange(option.code);
    close();
  }

  // keep the highlighted line in view inside the list (not the whole page)
  useEffect(() => {
    if (!open) return;
    const el = listRef.current?.children[active];
    if (el) scrollWithin(el);
  }, [open, active, options]);

  function onKeyDown(e) {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (!open) show();
        else setActive((a) => Math.min(a + 1, options.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        if (!open) show();
        else setActive((a) => Math.max(a - 1, 0));
        break;
      case "PageDown":
        if (open) {
          e.preventDefault();
          setActive((a) => Math.min(a + 8, options.length - 1));
        }
        break;
      case "PageUp":
        if (open) {
          e.preventDefault();
          setActive((a) => Math.max(a - 8, 0));
        }
        break;
      case "Enter":
        if (open) {
          e.preventDefault(); // do not submit the form behind the list
          choose(options[active]);
        }
        break;
      case "Escape":
        if (open) {
          e.preventDefault();
          e.stopPropagation();
          close();
        }
        break;
      case "Tab":
        close();
        break;
      default:
    }
  }

  return (
    <div className="cpick">
      <input
        id={inputId}
        className="field cpick__input"
        role="combobox"
        aria-label={ariaLabel}
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={open && options[active] ? `${inputId}-o${active}` : undefined}
        autoComplete="off"
        spellCheck={false}
        disabled={disabled}
        placeholder={open ? "Начните вводить название" : undefined}
        value={open ? query : selected}
        onFocus={show}
        onClick={show}
        onBlur={close}
        onKeyDown={onKeyDown}
        onChange={(e) => {
          setQuery(e.target.value);
          setActive(0);
          setOpen(true);
        }}
      />
      <span className={"cpick__chev" + (open ? " cpick__chev--open" : "")} aria-hidden="true">
        <Icon name="chevron" size={18} />
      </span>
      {open && (
        <ul className="cpick__list" id={listId} role="listbox" ref={listRef} aria-label={ariaLabel || "Страна"}>
          {options.map((c, i) => (
            <li
              key={c.code || "none"}
              id={`${inputId}-o${i}`}
              role="option"
              aria-selected={c.code === (value || "")}
              className={
                "cpick__opt" + (i === active ? " cpick__opt--active" : "") + (c.code === (value || "") ? " cpick__opt--selected" : "")
              }
              // mousedown, not click: the field would lose the focus (and close the list) before a click
              onMouseDown={(e) => {
                e.preventDefault();
                choose(c);
              }}
              onMouseMove={() => i !== active && setActive(i)}
            >
              <span className={c.code ? "" : "cpick__none"}>{c.ru}</span>
              {c.code && <span className="cpick__code">{c.code}</span>}
            </li>
          ))}
          {options.length === 0 && (
            <li className="cpick__empty" role="presentation">
              Ничего не найдено
            </li>
          )}
        </ul>
      )}
    </div>
  );
}
