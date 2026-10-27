// Russian display names for theme folders and board files. A name that is not
// listed here is shown as the folder name made readable ("my-set" -> "My set"),
// so a new set appears in the list even before it is translated: add a line
// here only to give it a Russian name. Proper names (authors, brands) stay.
export const NAMES_RU = {
  boards: {
    "blue-marble": "Голубой мрамор",
    blue: "Синий узор",
    blue2: "Синий узор 2",
    blue3: "Синий узор 3",
    brown: "Коричневый узор",
    canvas2: "Холст",
    "green-plastic": "Зелёный пластик",
    green: "Зелёный узор",
    grey: "Серый узор",
    ic: "IC",
    leather: "Кожа",
    maple: "Клён",
    maple2: "Клён 2",
    marble: "Мрамор",
    metal: "Металл",
    "ncf-board": "NCF",
    olive: "Оливковый узор",
    "pink-pyramid": "Розовая пирамида",
    "purple-diag": "Фиолетовая диагональ",
    purple: "Фиолетовый узор",
    wood: "Дерево 1",
    wood2: "Дерево 2",
    wood3: "Дерево 3",
    wood4: "Дерево 4",
  },
  pieces: {
    caliente: "Кальенте",
    california: "Калифорния",
    cardinal: "Кардинал",
    celtic: "Кельтские",
    companion: "Компаньон",
    disguised: "Замаскированные",
    fantasy: "Фэнтези",
    fresca: "Фреска",
    governor: "Губернатор",
    horsey: "Лошадка",
    leipzig: "Лейпциг",
    letter: "Буквы",
    maestro: "Маэстро",
    merida: "Мерида",
    "minimal-warmth": "Тёплый минимализм",
    monarchy: "Монархия",
    papercut: "Бумага",
    pixel: "Пиксели",
    riohacha: "Риоача",
    "shahi-ivory-brown": "Шахи, слоновая кость",
    shapes: "Фигуры-формы",
    spatial: "Пространственные",
    tatiana: "Татьяна",
  },
  sounds: {
    default: "Стандартные",
    futuristic: "Футуристические",
    nes: "NES, 8 бит",
    piano: "Пианино",
    robot: "Робот",
    sfx: "Эффекты",
    woodland: "Лес",
  },
};

// The name of a board, piece set, background, sound theme or preset in the language of the site
// (kind: "boards" | "pieces" | "backgrounds" | "sounds" | "presets"). The names are in the i18n
// dictionary (themes.<kind>.<id>); a set that is not there keeps its own name (the Russian one
// from NAMES_RU, or the folder name made readable).
export function themeName(t, kind, item) {
  let id = String(item.id);
  const folder = id.replace(/^(img|tiles):/, "");
  if (folder !== id) id = folder;
  else if (kind === "boards") id = "color." + id;
  const key = `themes.${kind}.${id}`;
  const s = t(key);
  return s === key ? item.name : s;
}
