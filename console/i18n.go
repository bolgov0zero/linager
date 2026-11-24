package console

import (
	"fmt"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/installer"
)

// tr returns the text for key in lang (English when the key is missing), formatted with args.
func tr(l installer.Lang, key string, args ...any) string {
	s, ok := dict[l][key]
	if !ok {
		s = dict[installer.EN][key]
	}
	if s == "" {
		s = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// span writes a duration roughly: "3 d 4 h", "5 h 12 min", "7 min".
func span(l installer.Lang, d time.Duration) string {
	if d < time.Minute {
		return tr(l, "u.lessmin")
	}
	days, hours, mins := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	var parts []string
	add := func(n int, unit string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, tr(l, unit)))
		}
	}
	switch {
	case days > 0:
		add(days, "u.d")
		add(hours, "u.h")
	case hours > 0:
		add(hours, "u.h")
		add(mins, "u.min")
	default:
		add(mins, "u.min")
	}
	return strings.Join(parts, " ")
}

func ago(l installer.Lang, d time.Duration) string {
	if d < time.Minute {
		return tr(l, "u.justnow")
	}
	return tr(l, "u.ago", span(l, d))
}

func join(parts ...string) string { return strings.Join(parts, " · ") }

var dict = map[installer.Lang]map[string]string{
	installer.RU: { //nolint:gosec // UI strings, not credentials
		"ver":         "версия %s",
		"mode":        "режим: %s",
		"role.server": "сервер",
		"role.agent":  "агент",
		"role.none":   "не установлено",
		"update.new":  "доступно обновление %s",
		"exit":        "Выход",
		"back":        "Назад",
		"sudo":        "sudo",

		"hint.menu":    "цифра или ↑↓ и Enter · q — выход",
		"hint.sub":     "цифра или ↑↓ и Enter · 0 или Esc — назад",
		"hint.confirm": "y — да, любая другая клавиша — отмена",
		"hint.form":    "Enter — далее · Esc — отмена",
		"hint.any":     "нажмите любую клавишу",
		"needroot":     "Часть данных скрыта от этого пользователя. Запустите «sudo linager».",

		"u.d": "д", "u.h": "ч", "u.min": "мин", "u.lessmin": "меньше минуты",
		"u.ago": "%s назад", "u.justnow": "только что",

		"s.service": "Служба", "s.db": "PostgreSQL", "s.admin": "Порт приложения", "s.agents": "Порт клиентов",
		"s.clients": "Клиенты", "s.backup": "Бэкап", "s.access": "Доступ к серверу", "s.update": "Версия",
		"s.server": "Сервер", "s.modules": "Модули", "s.agentver": "Версия",

		"v.loading": "…", "v.running": "работает", "v.since": "работает %s",
		"v.stopped": "остановлена", "v.failed": "сбой", "v.starting": "запускается", "v.missing": "не найдена",
		"v.unknown": "неизвестно", "v.dbok": "доступна", "v.dbbad": "недоступна: %s",
		"v.clients": "%d", "v.pending": "ждут подтверждения: %d", "v.pending.s": "ждут: %d", "v.nobackup": "нет",
		"v.access.off": "ограничение выключено", "v.access.white": "белый список", "v.access.black": "чёрный список",
		"v.uptodate": "последняя", "v.notenrolled": "не подключён", "v.modules": "%d из %d",
		"v.hidden": "нужен root",

		"c.card.hint": "Enter — открыть",
		"i.none":      "Linager не установлен на этом хосте. Что установить?",
		"i.hint":      "Установщик задаст нужные вопросы.",

		"m.s.update": "Обновить Linager", "m.s.backup": "Сделать бэкап", "m.s.restore": "Восстановить из бэкапа",
		"m.s.password": "Пароль администратора", "m.s.access": "Снять ограничение доступа к серверу",
		"m.s.fp": "Отпечаток сервера и подключение клиентов", "m.s.restart": "Перезапустить службу", "m.s.logs": "Логи",
		"m.a.enroll": "Подключить к серверу", "m.a.update": "Обновить агент", "m.a.restart": "Перезапустить службу",
		"m.a.logs": "Логи", "m.a.uninstall": "Удалить агент",
		"m.i.server": "Установить сервер", "m.i.agent": "Установить агент",

		"c.restore":   "Заменить базу Linager архивом %s? Сервер будет остановлен на время восстановления.",
		"c.access":    "Снять ограничение доступа к серверу (порт приложения)? Списки адресов сохранятся.",
		"c.restart":   "Перезапустить службу %s? Соединения будут разорваны.",
		"c.reenroll":  "Агент уже подключён. Подключить заново к другому или тому же серверу?",
		"t.restore":   "Выберите архив",
		"t.nobackup":  "В %s нет бэкапов (или нужен root).",
		"t.uninstall": "Удалить агент с этого хоста",
		"o.keep":      "Удалить агент, ключи оставить",
		"o.purge":     "Удалить агент вместе с ключами и настройкой",

		"f.pw.title": "Пароль администратора", "f.login": "Логин", "f.password": "Новый пароль", "f.repeat": "Повторите пароль",
		"f.enroll.title": "Подключение к серверу", "f.server": "Адрес клиентского порта (host:port)",
		"f.fp":       "Отпечаток сервера SHA256:… (можно пропустить)",
		"f.required": "Поле обязательно", "f.mismatch": "Пароли не совпадают",

		"i.fp.title": "Подключение клиентов", "i.fp.fp": "Отпечаток", "i.fp.port": "Клиентский порт",
		"i.fp.cmd": "На клиенте:",

		"pause":    "Нажмите Enter, чтобы вернуться в меню",
		"err.sudo": "Нужны права root, а sudo не найден. Запустите linager от root.",
		"err.root": "Недостаточно прав. Запустите «sudo linager».",
		"err.exec": "Не удалось запустить команду: %s",
	},
	installer.EN: {
		"ver":         "version %s",
		"mode":        "mode: %s",
		"role.server": "server",
		"role.agent":  "agent",
		"role.none":   "not installed",
		"update.new":  "update %s available",
		"exit":        "Exit",
		"back":        "Back",
		"sudo":        "sudo",

		"hint.menu":    "digit or ↑↓ and Enter · q to quit",
		"hint.sub":     "digit or ↑↓ and Enter · 0 or Esc to go back",
		"hint.confirm": "y to confirm, any other key cancels",
		"hint.form":    "Enter next · Esc cancel",
		"hint.any":     "press any key",
		"needroot":     "Some data is hidden from this user. Run «sudo linager».",

		"u.d": "d", "u.h": "h", "u.min": "min", "u.lessmin": "less than a minute",
		"u.ago": "%s ago", "u.justnow": "just now",

		"s.service": "Service", "s.db": "PostgreSQL", "s.admin": "Application port", "s.agents": "Client port",
		"s.clients": "Clients", "s.backup": "Backup", "s.access": "Server access", "s.update": "Version",
		"s.server": "Server", "s.modules": "Modules", "s.agentver": "Version",

		"v.loading": "…", "v.running": "running", "v.since": "running %s",
		"v.stopped": "stopped", "v.failed": "failed", "v.starting": "starting", "v.missing": "not found",
		"v.unknown": "unknown", "v.dbok": "reachable", "v.dbbad": "unreachable: %s",
		"v.clients": "%d", "v.pending": "waiting for approval: %d", "v.pending.s": "waiting: %d", "v.nobackup": "none",
		"v.access.off": "restriction is off", "v.access.white": "allow list", "v.access.black": "block list",
		"v.uptodate": "latest", "v.notenrolled": "not enrolled", "v.modules": "%d of %d",
		"v.hidden": "root needed",

		"c.card.hint": "Enter to open",
		"i.none":      "Linager is not installed on this host. What to install?",
		"i.hint":      "The installer asks for what it needs.",

		"m.s.update": "Update Linager", "m.s.backup": "Make a backup", "m.s.restore": "Restore from a backup",
		"m.s.password": "Administrator password", "m.s.access": "Turn off the server access restriction",
		"m.s.fp": "Server fingerprint and client enrollment", "m.s.restart": "Restart the service", "m.s.logs": "Logs",
		"m.a.enroll": "Connect to a server", "m.a.update": "Update the agent", "m.a.restart": "Restart the service",
		"m.a.logs": "Logs", "m.a.uninstall": "Remove the agent",
		"m.i.server": "Install the server", "m.i.agent": "Install the agent",

		"c.restore":   "Replace the Linager database with %s? The server is stopped during the restore.",
		"c.access":    "Turn off the server access restriction (the application port)? The address lists are kept.",
		"c.restart":   "Restart the service %s? Connections are dropped.",
		"c.reenroll":  "The agent is already enrolled. Enroll again, to the same or another server?",
		"t.restore":   "Pick an archive",
		"t.nobackup":  "There are no backups in %s (or root is needed).",
		"t.uninstall": "Remove the agent from this host",
		"o.keep":      "Remove the agent, keep its keys",
		"o.purge":     "Remove the agent with its keys and configuration",

		"f.pw.title": "Administrator password", "f.login": "Login", "f.password": "New password", "f.repeat": "Repeat the password",
		"f.enroll.title": "Connect to a server", "f.server": "Client port address (host:port)",
		"f.fp":       "Server fingerprint SHA256:… (optional)",
		"f.required": "This field is required", "f.mismatch": "The passwords do not match",

		"i.fp.title": "Enrolling clients", "i.fp.fp": "Fingerprint", "i.fp.port": "Client port",
		"i.fp.cmd": "On the client:",

		"pause":    "Press Enter to return to the menu",
		"err.sudo": "Root is needed and sudo was not found. Run linager as root.",
		"err.root": "Not enough rights. Run «sudo linager».",
		"err.exec": "Could not start the command: %s",
	},
}
