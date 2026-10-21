package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The open-profile fields (Settings -> Profile). All are optional and public.
const (
	maxPersonName = 40 // first and last name
	maxLocation   = 60
	maxAbout      = 500

	// fieldsChangeEvery is the least time between two saves of the profile fields (not the name,
	// which has its own, longer wait).
	fieldsChangeEvery = 5 * time.Second

	avatarLetter = ""       // the first letter of the name
	avatarGoogle = "google" // the photo Google sent
	presetPrefix = "preset:"
)

// avatarPresets are the ready-made avatars; the site draws them (web/src/auth/avatarPresets.js) and
// keeps its own copy of this list, which a test there keeps equal to the ids below.
var avatarPresets = []string{"crown", "star", "bolt", "flame", "trophy", "bot", "bots", "puzzles", "board", "coach", "openings", "duel"}

// resolveAvatar turns what the client asked for ("" the letter, "google", "preset:<id>") into the value
// kept in users.avatar_url. photo is the person's Google photo ("" if they have none). Anything else,
// a bare address included, is refused: a person cannot make the site load a picture from anywhere.
func resolveAvatar(choice, photo string) (string, error) {
	switch {
	case choice == avatarLetter:
		return "", nil
	case choice == avatarGoogle:
		if photo == "" {
			return "", errors.New("Фото Google доступно после входа через Google.")
		}
		return photo, nil
	case strings.HasPrefix(choice, presetPrefix):
		id := strings.TrimPrefix(choice, presetPrefix)
		for _, p := range avatarPresets {
			if p == id {
				return choice, nil
			}
		}
	}
	return "", errors.New("Такого аватара нет.")
}

// foldSpaces trims s and turns every run of spaces into one. A tab, a line break or any other
// non-printing character is an error (ok = false); only the ordinary and the no-break space count as
// spaces. Emoji and other symbols are printable and pass.
func foldSpaces(s string) (string, bool) {
	var b strings.Builder
	space := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == ' ' || r == ' ':
			space = true
			continue
		case !unicode.IsPrint(r):
			return "", false
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String(), true
}

// validPersonName checks a first or last name: letters (any alphabet) with space, hyphen, apostrophe and
// full stop between them, at most maxPersonName characters. "" clears the field. label is the field's
// name in the message ("Имя", "Фамилия").
func validPersonName(in, label string) (string, error) {
	out, ok := foldSpaces(in)
	bad := fmt.Errorf("В поле «%s» можно использовать буквы, пробел, дефис, апостроф и точку.", label)
	if !ok {
		return "", bad
	}
	letters := 0
	for _, r := range out {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsMark(r), r == ' ', r == '-', r == '.', r == '\'', r == '’':
		default:
			return "", bad
		}
	}
	if out != "" && letters == 0 {
		return "", fmt.Errorf("В поле «%s» должна быть хотя бы одна буква.", label)
	}
	if utf8.RuneCountInString(out) > maxPersonName {
		return "", fmt.Errorf("Поле «%s» слишком длинное: не больше %d символов.", label, maxPersonName)
	}
	return out, nil
}

// validLocation checks the free-text place ("Минск", "Гродно, Беларусь"): one line, at most
// maxLocation characters, no control characters. "" clears the field.
func validLocation(in string) (string, error) {
	out, ok := foldSpaces(in)
	if !ok {
		return "", errors.New("В поле «Местоположение» есть недопустимые символы: оно должно быть в одну строку.")
	}
	if utf8.RuneCountInString(out) > maxLocation {
		return "", fmt.Errorf("Поле «Местоположение» слишком длинное: не больше %d символов.", maxLocation)
	}
	return out, nil
}

// validAbout checks the "about me" text: plain text (the site shows it as text, never as markup),
// line breaks allowed but no more than one empty line in a row, at most maxAbout characters counted
// after that tidying. "" clears the field.
func validAbout(in string) (string, error) {
	in = strings.NewReplacer("\r\n", "\n", "\r", "\n", " ", " ").Replace(in)
	var lines []string
	blank := 0
	for _, line := range strings.Split(strings.TrimSpace(in), "\n") {
		line = strings.TrimRight(line, " ")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		for _, r := range line {
			if !unicode.IsPrint(r) && r != ' ' { // a tab, a control or an invisible direction mark
				return "", errors.New("В тексте «Обо мне» есть недопустимые символы.")
			}
		}
		lines = append(lines, line)
	}
	out := strings.Join(lines, "\n")
	if utf8.RuneCountInString(out) > maxAbout {
		return "", fmt.Errorf("Текст «Обо мне» слишком длинный: не больше %d символов.", maxAbout)
	}
	return out, nil
}
