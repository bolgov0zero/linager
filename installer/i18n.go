package installer

import (
	"fmt"
	"os"
	"strings"
)

// DetectLang picks the interface language from the environment (LC_ALL, LANG), English by default.
func DetectLang() Lang {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "ru") {
				return RU
			}
			return EN
		}
	}
	return EN
}

// ParseLang converts a flag value to a Lang; unknown values fall back to detection.
func ParseLang(s string) Lang {
	switch strings.ToLower(s) {
	case "ru":
		return RU
	case "en":
		return EN
	}
	return DetectLang()
}

// tr returns the localized string for key, formatted with args.
func tr(l Lang, key string, args ...any) string {
	s, ok := dict[l][key]
	if !ok {
		s = dict[EN][key]
	}
	if s == "" {
		s = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

var dict = map[Lang]map[string]string{
	RU: { //nolint:gosec // UI strings, not credentials
		"role.title":         "Установка Linager",
		"role.label":         "Что установить?",
		"role.server":        "Сервер",
		"role.agent":         "Клиент (агент)",
		"srv.title":          "Сервер",
		"srv.agentport":      "Порт для клиентов",
		"srv.agentport.h":    "Клиенты подключаются сюда напрямую (TLS)",
		"srv.appport":        "Порт приложения",
		"srv.appport.h":      "Сюда подключается приложение Linager (TLS)",
		"srv.user":           "Логин администратора",
		"srv.password":       "Пароль администратора",
		"srv.password.h":     "От 12 символов. Пусто — сгенерировать",
		"err.required":       "Обязательное поле",
		"err.port":           "Порт от 1 до 65535",
		"err.portbusy":       "Порт %d уже занят",
		"err.portsame":       "Порты должны отличаться",
		"err.domain":         "Введите имя домена, например linager.example.com",
		"err.password":       "Не короче 12 символов",
		"err.login":          "Только латинские буквы, цифры, . _ -",
		"agent.title":        "Клиент",
		"agent.server":       "Адрес сервера",
		"agent.server.h":     "host:порт клиентов, например 10.0.0.5:9443",
		"agent.fp":           "Отпечаток сервера",
		"agent.fp.trust":     "Доверять этому серверу?",
		"agent.yes":          "Да, доверять",
		"agent.no":           "Нет",
		"err.hostport":       "Формат host:порт",
		"st.check":           "Проверка системы",
		"st.packages":        "PostgreSQL",
		"st.database":        "База данных",
		"st.config":          "Конфигурация и ключи",
		"st.admin":           "Администратор",
		"st.service":         "Служба",
		"st.verify":          "Проверка запуска",
		"st.connect":         "Связь с сервером",
		"st.pair":            "Привязка к серверу",
		"st.binary":          "Установка программы",
		"info.apt":           "установка пакетов…",
		"info.update":        "обновление списка пакетов…",
		"info.start":         "запуск…",
		"info.wait":          "ожидание…",
		"done.server":        "Сервер установлен",
		"done.agent":         "Клиент установлен и подключён",
		"fail.title":         "Установка не завершена",
		"sum.admin":          "Приложение (адрес)",
		"sum.login":          "Логин",
		"sum.password":       "Пароль",
		"sum.password.gen":   "сгенерирован, показан один раз",
		"sum.fp":             "Отпечаток для клиентов",
		"sum.agentport":      "Порт клиентов",
		"sum.appport":        "Порт приложения",
		"sum.client":         "Установка клиента",
		"sum.log":            "Журнал",
		"sum.server":         "Сервер",
		"sum.state":          "Состояние",
		"sum.state.ok":       "подключён и работает как служба",
		"note.nft":           "nftables активен: откройте порты клиентов и приложения, например: nft add rule inet filter input tcp dport { %d, %d } accept",
		"note.app":           "Откройте приложение Linager, введите %s и сверьте отпечаток сертификата с показанным выше. Интерфейса в браузере нет: управление через приложение.",
		"note.already":       "Linager уже установлен (%s). Для переустановки добавьте --force.",
		"err.root":           "Запустите от root: sudo linager install",
		"err.os":             "Неподдерживаемая система: %s (нужны Debian 12/13, Ubuntu 22.04/24.04/26.04)",
		"err.systemd":        "Нужен systemd",
		"err.pglisten":       "PostgreSQL слушает не только localhost (listen_addresses=%s). Установите listen_addresses = 'localhost' и повторите.",
		"err.pghba":          "В pg_hba.conf есть правила trust/md5 для TCP. Разрешён только scram-sha-256.",
		"err.fp":             "Отпечаток сервера не совпал: получен %s, ожидался %s",
		"err.fpmissing":      "Нужен --fingerprint (или интерактивный запуск)",
		"err.notrust":        "Сервер не подтверждён",
		"err.enrolled":       "Этот хост уже привязан (%s). Удалите клиента в приложении и выполните: linager agent uninstall --purge, либо добавьте --force.",
		"err.noninteractive": "Не хватает параметров для неинтерактивного режима: %s",
	},
	EN: { //nolint:gosec // UI strings, not credentials
		"role.title":         "Install Linager",
		"role.label":         "What to install?",
		"role.server":        "Server",
		"role.agent":         "Client (agent)",
		"srv.title":          "Server",
		"srv.agentport":      "Port for clients",
		"srv.agentport.h":    "Clients connect here directly (TLS)",
		"srv.appport":        "Application port",
		"srv.appport.h":      "The Linager application connects here (TLS)",
		"srv.user":           "Administrator login",
		"srv.password":       "Administrator password",
		"srv.password.h":     "At least 12 characters. Empty to generate",
		"err.required":       "Required",
		"err.port":           "Port 1-65535",
		"err.portbusy":       "Port %d is already in use",
		"err.portsame":       "Ports must differ",
		"err.domain":         "Enter a domain name, e.g. linager.example.com",
		"err.password":       "At least 12 characters",
		"err.login":          "Latin letters, digits, . _ - only",
		"agent.title":        "Client",
		"agent.server":       "Server address",
		"agent.server.h":     "host:client-port, e.g. 10.0.0.5:9443",
		"agent.fp":           "Server fingerprint",
		"agent.fp.trust":     "Trust this server?",
		"agent.yes":          "Yes, trust",
		"agent.no":           "No",
		"err.hostport":       "Use host:port",
		"st.check":           "System check",
		"st.packages":        "PostgreSQL",
		"st.database":        "Database",
		"st.config":          "Configuration and keys",
		"st.admin":           "Administrator",
		"st.service":         "Service",
		"st.verify":          "Startup check",
		"st.connect":         "Server connection",
		"st.pair":            "Pairing",
		"st.binary":          "Installing program",
		"info.apt":           "installing packages…",
		"info.update":        "updating package lists…",
		"info.start":         "starting…",
		"info.wait":          "waiting…",
		"done.server":        "Server installed",
		"done.agent":         "Client installed and connected",
		"fail.title":         "Installation failed",
		"sum.admin":          "Application (address)",
		"sum.login":          "Login",
		"sum.password":       "Password",
		"sum.password.gen":   "generated, shown once",
		"sum.fp":             "Fingerprint for clients",
		"sum.agentport":      "Client port",
		"sum.appport":        "Application port",
		"sum.client":         "Install a client",
		"sum.log":            "Log",
		"sum.server":         "Server",
		"sum.state":          "State",
		"sum.state.ok":       "connected and running as a service",
		"note.nft":           "nftables is active: open the client and application ports, e.g.: nft add rule inet filter input tcp dport { %d, %d } accept",
		"note.app":           "Open the Linager application, enter %s and compare the certificate fingerprint with the one above. There is no browser interface: the server is managed with the application.",
		"note.already":       "Linager is already installed (%s). Add --force to reinstall.",
		"err.root":           "Run as root: sudo linager install",
		"err.os":             "Unsupported system: %s (Debian 12/13, Ubuntu 22.04/24.04/26.04 required)",
		"err.systemd":        "systemd is required",
		"err.pglisten":       "PostgreSQL listens beyond localhost (listen_addresses=%s). Set listen_addresses = 'localhost' and retry.",
		"err.pghba":          "pg_hba.conf has trust/md5 rules for TCP. Only scram-sha-256 is allowed.",
		"err.fp":             "Server fingerprint mismatch: got %s, expected %s",
		"err.fpmissing":      "--fingerprint is required (or run interactively)",
		"err.notrust":        "Server was not confirmed",
		"err.enrolled":       "This host is already paired (%s). Delete the client in the application and run: linager agent uninstall --purge, or add --force.",
		"err.noninteractive": "Missing parameters for non-interactive mode: %s",
	},
}
