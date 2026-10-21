// Client of the lessons API (backend/internal/lessons).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    const e = new Error(body?.error || `Сервер ответил ${res.status}`);
    e.status = res.status;
    throw e;
  }
  return body;
}

// { sections: [{ id, title, description, lessons: [{ id, title, description, icon, minutes, steps,
//   exercises, progress?: { step, mistakes, completed } }] }], total, completed, next? }
export async function fetchCatalog() {
  return readJSON(await fetch("/api/lessons"));
}

// One lesson with all its steps (see lesson.js for the shape), and `progress` for a signed-in person.
export async function fetchLesson(id) {
  return readJSON(await fetch(`/api/lessons/${encodeURIComponent(id)}`));
}

// Saves how far the person has got: `step` steps done, `mistakes` wrong moves, `done` at the end.
// Needs a signed-in person (a guest will do). Answers with the stored progress.
export async function saveProgress(id, body) {
  const res = await fetch(`/api/lessons/${encodeURIComponent(id)}/progress`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return readJSON(res);
}
