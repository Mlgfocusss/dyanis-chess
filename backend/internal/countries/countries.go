// Package countries holds the list of countries a person can name as theirs: the officially assigned
// ISO 3166-1 alpha-2 codes with Russian and English names. It is the only source of the list; the
// site fetches it from GET /api/countries and the server accepts only these codes (users.country).
// The table is compiled in: no outside service is asked.
package countries

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// Country is one entry: Code is two capital Latin letters.
type Country struct {
	Code string `json:"code"`
	Ru   string `json:"ru"`
	En   string `json:"en"`
}

// table is in the order of the codes; List sorts a copy by the Russian name.
var table = []Country{
	{"AD", "Андорра", "Andorra"},
	{"AE", "Объединённые Арабские Эмираты", "United Arab Emirates"},
	{"AF", "Афганистан", "Afghanistan"},
	{"AG", "Антигуа и Барбуда", "Antigua and Barbuda"},
	{"AI", "Ангилья", "Anguilla"},
	{"AL", "Албания", "Albania"},
	{"AM", "Армения", "Armenia"},
	{"AO", "Ангола", "Angola"},
	{"AQ", "Антарктида", "Antarctica"},
	{"AR", "Аргентина", "Argentina"},
	{"AS", "Американское Самоа", "American Samoa"},
	{"AT", "Австрия", "Austria"},
	{"AU", "Австралия", "Australia"},
	{"AW", "Аруба", "Aruba"},
	{"AX", "Аландские острова", "Åland Islands"},
	{"AZ", "Азербайджан", "Azerbaijan"},
	{"BA", "Босния и Герцеговина", "Bosnia and Herzegovina"},
	{"BB", "Барбадос", "Barbados"},
	{"BD", "Бангладеш", "Bangladesh"},
	{"BE", "Бельгия", "Belgium"},
	{"BF", "Буркина-Фасо", "Burkina Faso"},
	{"BG", "Болгария", "Bulgaria"},
	{"BH", "Бахрейн", "Bahrain"},
	{"BI", "Бурунди", "Burundi"},
	{"BJ", "Бенин", "Benin"},
	{"BL", "Сен-Бартелеми", "Saint Barthélemy"},
	{"BM", "Бермудские острова", "Bermuda"},
	{"BN", "Бруней", "Brunei"},
	{"BO", "Боливия", "Bolivia"},
	{"BQ", "Бонэйр, Синт-Эстатиус и Саба", "Bonaire, Sint Eustatius and Saba"},
	{"BR", "Бразилия", "Brazil"},
	{"BS", "Багамы", "Bahamas"},
	{"BT", "Бутан", "Bhutan"},
	{"BV", "Остров Буве", "Bouvet Island"},
	{"BW", "Ботсвана", "Botswana"},
	{"BY", "Беларусь", "Belarus"},
	{"BZ", "Белиз", "Belize"},
	{"CA", "Канада", "Canada"},
	{"CC", "Кокосовые острова", "Cocos (Keeling) Islands"},
	{"CD", "Демократическая Республика Конго", "Democratic Republic of the Congo"},
	{"CF", "Центральноафриканская Республика", "Central African Republic"},
	{"CG", "Республика Конго", "Republic of the Congo"},
	{"CH", "Швейцария", "Switzerland"},
	{"CI", "Кот-д’Ивуар", "Côte d'Ivoire"},
	{"CK", "Острова Кука", "Cook Islands"},
	{"CL", "Чили", "Chile"},
	{"CM", "Камерун", "Cameroon"},
	{"CN", "Китай", "China"},
	{"CO", "Колумбия", "Colombia"},
	{"CR", "Коста-Рика", "Costa Rica"},
	{"CU", "Куба", "Cuba"},
	{"CV", "Кабо-Верде", "Cabo Verde"},
	{"CW", "Кюрасао", "Curaçao"},
	{"CX", "Остров Рождества", "Christmas Island"},
	{"CY", "Кипр", "Cyprus"},
	{"CZ", "Чехия", "Czechia"},
	{"DE", "Германия", "Germany"},
	{"DJ", "Джибути", "Djibouti"},
	{"DK", "Дания", "Denmark"},
	{"DM", "Доминика", "Dominica"},
	{"DO", "Доминиканская Республика", "Dominican Republic"},
	{"DZ", "Алжир", "Algeria"},
	{"EC", "Эквадор", "Ecuador"},
	{"EE", "Эстония", "Estonia"},
	{"EG", "Египет", "Egypt"},
	{"EH", "Западная Сахара", "Western Sahara"},
	{"ER", "Эритрея", "Eritrea"},
	{"ES", "Испания", "Spain"},
	{"ET", "Эфиопия", "Ethiopia"},
	{"FI", "Финляндия", "Finland"},
	{"FJ", "Фиджи", "Fiji"},
	{"FK", "Фолклендские острова", "Falkland Islands"},
	{"FM", "Микронезия", "Micronesia"},
	{"FO", "Фарерские острова", "Faroe Islands"},
	{"FR", "Франция", "France"},
	{"GA", "Габон", "Gabon"},
	{"GB", "Великобритания", "United Kingdom"},
	{"GD", "Гренада", "Grenada"},
	{"GE", "Грузия", "Georgia"},
	{"GF", "Французская Гвиана", "French Guiana"},
	{"GG", "Гернси", "Guernsey"},
	{"GH", "Гана", "Ghana"},
	{"GI", "Гибралтар", "Gibraltar"},
	{"GL", "Гренландия", "Greenland"},
	{"GM", "Гамбия", "Gambia"},
	{"GN", "Гвинея", "Guinea"},
	{"GP", "Гваделупа", "Guadeloupe"},
	{"GQ", "Экваториальная Гвинея", "Equatorial Guinea"},
	{"GR", "Греция", "Greece"},
	{"GS", "Южная Георгия и Южные Сандвичевы острова", "South Georgia and the South Sandwich Islands"},
	{"GT", "Гватемала", "Guatemala"},
	{"GU", "Гуам", "Guam"},
	{"GW", "Гвинея-Бисау", "Guinea-Bissau"},
	{"GY", "Гайана", "Guyana"},
	{"HK", "Гонконг", "Hong Kong"},
	{"HM", "Остров Херд и острова Макдональд", "Heard Island and McDonald Islands"},
	{"HN", "Гондурас", "Honduras"},
	{"HR", "Хорватия", "Croatia"},
	{"HT", "Гаити", "Haiti"},
	{"HU", "Венгрия", "Hungary"},
	{"ID", "Индонезия", "Indonesia"},
	{"IE", "Ирландия", "Ireland"},
	{"IL", "Израиль", "Israel"},
	{"IM", "Остров Мэн", "Isle of Man"},
	{"IN", "Индия", "India"},
	{"IO", "Британская территория в Индийском океане", "British Indian Ocean Territory"},
	{"IQ", "Ирак", "Iraq"},
	{"IR", "Иран", "Iran"},
	{"IS", "Исландия", "Iceland"},
	{"IT", "Италия", "Italy"},
	{"JE", "Джерси", "Jersey"},
	{"JM", "Ямайка", "Jamaica"},
	{"JO", "Иордания", "Jordan"},
	{"JP", "Япония", "Japan"},
	{"KE", "Кения", "Kenya"},
	{"KG", "Киргизия", "Kyrgyzstan"},
	{"KH", "Камбоджа", "Cambodia"},
	{"KI", "Кирибати", "Kiribati"},
	{"KM", "Коморы", "Comoros"},
	{"KN", "Сент-Китс и Невис", "Saint Kitts and Nevis"},
	{"KP", "Северная Корея", "North Korea"},
	{"KR", "Южная Корея", "South Korea"},
	{"KW", "Кувейт", "Kuwait"},
	{"KY", "Острова Кайман", "Cayman Islands"},
	{"KZ", "Казахстан", "Kazakhstan"},
	{"LA", "Лаос", "Laos"},
	{"LB", "Ливан", "Lebanon"},
	{"LC", "Сент-Люсия", "Saint Lucia"},
	{"LI", "Лихтенштейн", "Liechtenstein"},
	{"LK", "Шри-Ланка", "Sri Lanka"},
	{"LR", "Либерия", "Liberia"},
	{"LS", "Лесото", "Lesotho"},
	{"LT", "Литва", "Lithuania"},
	{"LU", "Люксембург", "Luxembourg"},
	{"LV", "Латвия", "Latvia"},
	{"LY", "Ливия", "Libya"},
	{"MA", "Марокко", "Morocco"},
	{"MC", "Монако", "Monaco"},
	{"MD", "Молдова", "Moldova"},
	{"ME", "Черногория", "Montenegro"},
	{"MF", "Сен-Мартен", "Saint Martin"},
	{"MG", "Мадагаскар", "Madagascar"},
	{"MH", "Маршалловы Острова", "Marshall Islands"},
	{"MK", "Северная Македония", "North Macedonia"},
	{"ML", "Мали", "Mali"},
	{"MM", "Мьянма", "Myanmar"},
	{"MN", "Монголия", "Mongolia"},
	{"MO", "Макао", "Macao"},
	{"MP", "Северные Марианские острова", "Northern Mariana Islands"},
	{"MQ", "Мартиника", "Martinique"},
	{"MR", "Мавритания", "Mauritania"},
	{"MS", "Монтсеррат", "Montserrat"},
	{"MT", "Мальта", "Malta"},
	{"MU", "Маврикий", "Mauritius"},
	{"MV", "Мальдивы", "Maldives"},
	{"MW", "Малави", "Malawi"},
	{"MX", "Мексика", "Mexico"},
	{"MY", "Малайзия", "Malaysia"},
	{"MZ", "Мозамбик", "Mozambique"},
	{"NA", "Намибия", "Namibia"},
	{"NC", "Новая Каледония", "New Caledonia"},
	{"NE", "Нигер", "Niger"},
	{"NF", "Остров Норфолк", "Norfolk Island"},
	{"NG", "Нигерия", "Nigeria"},
	{"NI", "Никарагуа", "Nicaragua"},
	{"NL", "Нидерланды", "Netherlands"},
	{"NO", "Норвегия", "Norway"},
	{"NP", "Непал", "Nepal"},
	{"NR", "Науру", "Nauru"},
	{"NU", "Ниуэ", "Niue"},
	{"NZ", "Новая Зеландия", "New Zealand"},
	{"OM", "Оман", "Oman"},
	{"PA", "Панама", "Panama"},
	{"PE", "Перу", "Peru"},
	{"PF", "Французская Полинезия", "French Polynesia"},
	{"PG", "Папуа — Новая Гвинея", "Papua New Guinea"},
	{"PH", "Филиппины", "Philippines"},
	{"PK", "Пакистан", "Pakistan"},
	{"PL", "Польша", "Poland"},
	{"PM", "Сен-Пьер и Микелон", "Saint Pierre and Miquelon"},
	{"PN", "Острова Питкэрн", "Pitcairn Islands"},
	{"PR", "Пуэрто-Рико", "Puerto Rico"},
	{"PS", "Палестина", "Palestine"},
	{"PT", "Португалия", "Portugal"},
	{"PW", "Палау", "Palau"},
	{"PY", "Парагвай", "Paraguay"},
	{"QA", "Катар", "Qatar"},
	{"RE", "Реюньон", "Réunion"},
	{"RO", "Румыния", "Romania"},
	{"RS", "Сербия", "Serbia"},
	{"RU", "Россия", "Russia"},
	{"RW", "Руанда", "Rwanda"},
	{"SA", "Саудовская Аравия", "Saudi Arabia"},
	{"SB", "Соломоновы Острова", "Solomon Islands"},
	{"SC", "Сейшельские Острова", "Seychelles"},
	{"SD", "Судан", "Sudan"},
	{"SE", "Швеция", "Sweden"},
	{"SG", "Сингапур", "Singapore"},
	{"SH", "Остров Святой Елены", "Saint Helena, Ascension and Tristan da Cunha"},
	{"SI", "Словения", "Slovenia"},
	{"SJ", "Шпицберген и Ян-Майен", "Svalbard and Jan Mayen"},
	{"SK", "Словакия", "Slovakia"},
	{"SL", "Сьерра-Леоне", "Sierra Leone"},
	{"SM", "Сан-Марино", "San Marino"},
	{"SN", "Сенегал", "Senegal"},
	{"SO", "Сомали", "Somalia"},
	{"SR", "Суринам", "Suriname"},
	{"SS", "Южный Судан", "South Sudan"},
	{"ST", "Сан-Томе и Принсипи", "Sao Tome and Principe"},
	{"SV", "Сальвадор", "El Salvador"},
	{"SX", "Синт-Мартен", "Sint Maarten"},
	{"SY", "Сирия", "Syria"},
	{"SZ", "Эсватини", "Eswatini"},
	{"TC", "Острова Тёркс и Кайкос", "Turks and Caicos Islands"},
	{"TD", "Чад", "Chad"},
	{"TF", "Французские Южные территории", "French Southern Territories"},
	{"TG", "Того", "Togo"},
	{"TH", "Таиланд", "Thailand"},
	{"TJ", "Таджикистан", "Tajikistan"},
	{"TK", "Токелау", "Tokelau"},
	{"TL", "Восточный Тимор", "Timor-Leste"},
	{"TM", "Туркменистан", "Turkmenistan"},
	{"TN", "Тунис", "Tunisia"},
	{"TO", "Тонга", "Tonga"},
	{"TR", "Турция", "Türkiye"},
	{"TT", "Тринидад и Тобаго", "Trinidad and Tobago"},
	{"TV", "Тувалу", "Tuvalu"},
	{"TW", "Тайвань", "Taiwan"},
	{"TZ", "Танзания", "Tanzania"},
	{"UA", "Украина", "Ukraine"},
	{"UG", "Уганда", "Uganda"},
	{"UM", "Внешние малые острова США", "United States Minor Outlying Islands"},
	{"US", "США", "United States"},
	{"UY", "Уругвай", "Uruguay"},
	{"UZ", "Узбекистан", "Uzbekistan"},
	{"VA", "Ватикан", "Vatican City"},
	{"VC", "Сент-Винсент и Гренадины", "Saint Vincent and the Grenadines"},
	{"VE", "Венесуэла", "Venezuela"},
	{"VG", "Британские Виргинские острова", "British Virgin Islands"},
	{"VI", "Виргинские острова США", "United States Virgin Islands"},
	{"VN", "Вьетнам", "Vietnam"},
	{"VU", "Вануату", "Vanuatu"},
	{"WF", "Уоллис и Футуна", "Wallis and Futuna"},
	{"WS", "Самоа", "Samoa"},
	{"YE", "Йемен", "Yemen"},
	{"YT", "Майотта", "Mayotte"},
	{"ZA", "ЮАР", "South Africa"},
	{"ZM", "Замбия", "Zambia"},
	{"ZW", "Зимбабве", "Zimbabwe"},
}

var (
	sorted []Country
	byCode map[string]Country
	body   []byte // the JSON of List, made once
)

func init() {
	sorted = append([]Country(nil), table...)
	// By the Russian name; "ё" sorts as "е" so that Cyrillic order is the dictionary one.
	key := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "ё", "е"), "Ё", "Е"))
	}
	sort.SliceStable(sorted, func(i, j int) bool { return key(sorted[i].Ru) < key(sorted[j].Ru) })
	byCode = make(map[string]Country, len(table))
	for _, c := range table {
		byCode[c.Code] = c
	}
	body, _ = json.Marshal(map[string]any{"countries": sorted})
}

// List returns every country, sorted by Russian name. The slice must not be changed.
func List() []Country { return sorted }

// Valid reports whether code is one of the listed codes (exactly, in capitals).
func Valid(code string) bool {
	_, ok := byCode[code]
	return ok
}

// Lookup returns the country with this code.
func Lookup(code string) (Country, bool) {
	c, ok := byCode[code]
	return c, ok
}

// NameRu is the Russian name of the country, "" for an unknown code.
func NameRu(code string) string { return byCode[code].Ru }

// Handler serves GET /api/countries: {"countries":[{code, ru, en}, ...]}, public. The list changes
// only with a new build, so the browser may keep it for a day.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/countries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(body)
	})
	return mux
}
