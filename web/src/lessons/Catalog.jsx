import { useEffect, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { fetchCatalog } from "./api.js";
import { continueLesson, courseProgress, lessonPercent, lessonStatus, minutesLabel, sectionProgress, stepsLabel } from "./lesson.js";

// The list of lessons by section, with the progress of a signed-in person.
export default function Catalog() {
  const { status } = useAuth();
  const [catalog, setCatalog] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (status === "loading") return undefined;
    let alive = true;
    fetchCatalog().then(
      (c) => alive && setCatalog(c),
      (e) => alive && setError(status === "unavailable" ? "Уроки недоступны: сервер запущен без базы данных." : e.message)
    );
    return () => {
      alive = false;
    };
  }, [status]);

  const course = courseProgress(catalog);
  const next = continueLesson(catalog);
  const started = next && lessonStatus(next.progress) === "started";

  return (
    <div className="page lessons">
      <header className="page__head">
        <h1 className="page__title">Уроки</h1>
        <p className="page__sub">Короткие уроки с упражнениями на доске: от первых ходов фигур до матовых комбинаций.</p>
      </header>

      {error && <div className="banner banner--error">{error}</div>}
      {!catalog && !error && <p className="lessons__loading">Загружаю уроки…</p>}

      {catalog && (
        <>
          <section className="card lessons__course" aria-label="Общий прогресс">
            <div className="lessons__course-text">
              <h2>
                Пройдено {course.done} из {course.total}
              </h2>
              <div className="lbar" role="progressbar" aria-valuemin={0} aria-valuemax={course.total} aria-valuenow={course.done}>
                <div className="lbar__fill" style={{ width: `${course.percent}%` }} />
              </div>
            </div>
            {next ? (
              <a className="btn btn--primary lessons__go" href={`#lessons/${next.id}`}>
                <Icon name="play" size={16} />
                {started ? "Продолжить" : course.done ? "Следующий урок" : "Начать обучение"}
              </a>
            ) : (
              <span className="lessons__all">Все уроки пройдены</span>
            )}
          </section>

          {catalog.sections.map((s) => {
            const sp = sectionProgress(s);
            return (
              <section key={s.id} className="lessons__section" aria-labelledby={`sec-${s.id}`}>
                <div className="lessons__head">
                  <h2 id={`sec-${s.id}`}>{s.title}</h2>
                  <span className="lessons__count">
                    {sp.done} из {sp.total}
                  </span>
                </div>
                <p className="lessons__desc">{s.description}</p>
                <ul className="lessons__grid">
                  {s.lessons.map((l) => (
                    <li key={l.id}>
                      <LessonCard lesson={l} />
                    </li>
                  ))}
                </ul>
              </section>
            );
          })}
        </>
      )}
    </div>
  );
}

function LessonCard({ lesson: l }) {
  const st = lessonStatus(l.progress);
  const percent = lessonPercent(l.progress, l.steps);
  return (
    <a className={`card lcard lcard--${st}`} href={`#lessons/${l.id}`}>
      <span className="lcard__top">
        <span className="lcard__ico">
          <Icon name={l.icon} size={22} />
        </span>
        {st === "done" && (
          <span className="lcard__done">
            <Icon name="check" size={14} /> Пройден
          </span>
        )}
      </span>
      <span className="lcard__title">{l.title}</span>
      <span className="lcard__desc">{l.description}</span>
      <span className="lcard__meta">
        {minutesLabel(l.minutes)} · {stepsLabel(l.steps)}
      </span>
      <span className="lbar lbar--thin" aria-hidden="true">
        <span className="lbar__fill" style={{ width: `${percent}%` }} />
      </span>
      <span className="lcard__cta">{st === "started" ? "Продолжить" : st === "done" ? "Пройти ещё раз" : "Начать"}</span>
    </a>
  );
}
