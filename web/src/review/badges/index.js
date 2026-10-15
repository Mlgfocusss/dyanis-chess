// Pictures of the move-quality badges, if the author has drawn them: any file
// in this folder named after a class replaces the badge the site draws itself.
// See README.md in this folder for the names. Classes without a file keep the
// drawn badge, so the folder can be filled one picture at a time.
const files = import.meta.glob("./*.{svg,png,webp,jpg,jpeg,gif,avif}", {
  eager: true,
  query: "?url",
  import: "default",
});

export const BADGE_IMAGES = {};
for (const [path, url] of Object.entries(files)) {
  const name = path.replace(/^\.\//, "").replace(/\.[^.]+$/, "").toLowerCase();
  BADGE_IMAGES[name] = url;
}
