// A row of mutually exclusive choices; options are [value, label] pairs.
// Styles (.segmented) live in settings/settings.css.
export default function Segmented({ value, onChange, options, label }) {
  return (
    <div className="segmented" role="radiogroup" aria-label={label}>
      {options.map(([v, text]) => (
        <button
          key={v}
          type="button"
          role="radio"
          aria-checked={value === v}
          className={`segmented__opt${value === v ? " segmented__opt--on" : ""}`}
          onClick={() => onChange(v)}
        >
          {text}
        </button>
      ))}
    </div>
  );
}
