import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import { countryName, placeLine } from "../countries/countryList.js";
import useCountries from "../countries/useCountries.js";
import Icon from "../ui/Icon.jsx";
import RelationBar from "../community/RelationBar.jsx";
import RecentAchievements from "../achievements/RecentAchievements.jsx";
import { fetchUser } from "./api.js";
import { botsLine, joinedText, kindLabel, leadersHash, profileRatings, userFromHash } from "./leaders.js";
import { useLang, useT } from "../i18n/index.js";
import "./leaders.css";

// The public profile of another player (#user/<id>): who they are, ratings with their places in the
// tables, games and the points against the bots. Nothing private is sent by the server.
export default function UserPage() {
  const [target, setTarget] = useState(() => userFromHash(window.location.hash));
  useEffect(() => {
    const onHash = () => {
      const t = userFromHash(window.location.hash);
      setTarget((old) => (old.id === t.id && old.from === t.from && old.country === t.country ? old : t));
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return <UserView key={target.id} id={target.id} from={target.from} country={target.country} />;
}

function UserView({ id, from, country }) {
  const t = useT();
  const { user } = useAuth();
  const lang = useLang();
  const { countries } = useCountries();
  const [state, setState] = useState({ profile: null, error: "", missing: false });

  useEffect(() => {
    if (!id) {
      setState({ profile: null, error: "", missing: true });
      return undefined;
    }
    const ctl = new AbortController();
    fetchUser(id, ctl.signal).then(
      (profile) => setState({ profile, error: "", missing: false }),
      (e) => {
        if (e.name === "AbortError") return;
        setState({ profile: null, error: e.status === 404 ? "" : e.message, missing: e.status === 404 });
      },
    );
    return () => ctl.abort();
  }, [id]);

  const { profile, error, missing } = state;
  const back = (
    <a className="btn btn--ghost uprof__back" href={leadersHash(from, country)}>
      <Icon name="back" size={18} />
      {t("userPage.1")}
    </a>
  );

  if (missing || error) {
    return (
      <div className="page uprof">
        <div className="card uprof__empty">
          <h1>{missing ? t("userPage.2") : t("userPage.3")}</h1>
          <p>{missing ? t("userPage.4") : error}</p>
          {back}
        </div>
      </div>
    );
  }
  if (!profile) {
    return (
      <div className="page uprof">
        <p className="leaders__note">{t("leadersPage.loading")}</p>
      </div>
    );
  }

  const mine = user && user.id === profile.id;
  const since = joinedText(profile.createdAt);
  const cname = (profile.country && countries.length ? countryName(countries, profile.country, lang) : "") || profile.countryName;
  const place = placeLine(profile.location, cname);
  const ratings = profileRatings(profile);
  return (
    <div className="page uprof">
      <div className="uprof__bar">{back}</div>

      <section className="card uprof__head">
        <Avatar user={profile} size={72} />
        <div className="uprof__who">
          <h1 className="uprof__name" title={profile.name}>
            {profile.name}
            {mine && <span className="leaders__you">{t("leadersPage.13")}</span>}
          </h1>
          {profile.fullName && <p className="uprof__real">{profile.fullName}</p>}
          {place && (
            <p className="uprof__place-line">
              {profile.country && <span className="uprof__code" title={cname}>{profile.country}</span>}
              {place}
            </p>
          )}
          <p className="uprof__meta">
            {kindLabel(profile.kind)}
            {since && t("profilePanel.6", { since })}
          </p>
        </div>
        {mine && (
          <a className="btn btn--ghost" href="#profile">
            {t("userPage.8")}
          </a>
        )}
      </section>

      <RelationBar person={profile} />

      {profile.about && (
        <section className="card uprof__card">
          <header>
            <h2>{t("profilePanel.19")}</h2>
          </header>
          {/* plain text: React escapes it, the line breaks are kept by CSS */}
          <p className="uprof__about">{profile.about}</p>
        </section>
      )}

      <section className="card uprof__card">
        <header>
          <h2>{t("profile.ratingsTitle")}</h2>
          <span className="uprof__hint">{t("userPage.11")}</span>
        </header>
        <ul className="uprof__ratings">
          {ratings.map((r) => (
            <li key={r.category} className="uprof__rating">
              <span className="uprof__rname">{r.name}</span>
              <b className="uprof__rvalue">{r.value}</b>
              <span className="uprof__rsub">{r.record}</span>
              {r.place > 0 ? (
                <a className="uprof__place" href={leadersHash(r.category)}>
                  <Icon name="trophy" size={14} />
                  {r.placeLine}
                </a>
              ) : (
                <span className="uprof__rsub">{r.games ? t("userPage.12") : " "}</span>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="card uprof__card">
        <header>
          <h2>{t("userPage.13")}</h2>
        </header>
        <dl className="uprof__stats">
          <div>
            <dt>{t("userPage.14")}</dt>
            <dd>{profile.games}</dd>
          </div>
          <div>
            <dt>{t("userPage.15")}</dt>
            <dd className="uprof__points">
              <Icon name="star" size={18} />
              {profile.bots?.points || 0}
            </dd>
          </div>
          <div>
            <dt>{t("avatar.bots")}</dt>
            <dd>{botsLine(profile.bots) || "—"}</dd>
          </div>
        </dl>
      </section>

      <RecentAchievements userId={profile.id} mine={mine} />
    </div>
  );
}
