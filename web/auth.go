// Package web — Basic Auth для веб-дашборда.
// Учётные данные читаются из секции "web" в config.json.
// Пустой пароль = авторизация выключена (с предупреждением в консоли).
package web

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	)

type webAuthConfig struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type webRootConfig struct {
	Web webAuthConfig `json:"web"`
}

// loadAuth возвращает логин и пароль для Basic Auth.
// Пустой пароль означает, что авторизация отключена.
func loadAuth() (string, string) {
	raw, err := os.ReadFile("config.json")
	if err != nil {
		return "", ""
	}
	var cfg webRootConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", ""
	}
	return cfg.Web.Login, cfg.Web.Password
}

// withBasicAuth оборачивает HTTP-обработчик проверкой Basic Auth.
// Если пароль не задан в config.json — возвращает обработчик как есть.
func withBasicAuth(next http.Handler) http.Handler {
	login, password := loadAuth()

	if password == "" {
		fmt.Println("[i] ВНИМАНИЕ: веб-интерфейс работает БЕЗ авторизации.")
		fmt.Println("    Задайте пароль в config.json: \"web\": {\"login\": \"admin\", \"password\": \"...\"}")
		return next
	}

	if login == "" {
		login = "admin" // логин по умолчанию, если указан только пароль
	}

	fmt.Printf("[✓] Веб-интерфейс защищён Basic Auth (логин: %s)\n", login)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(login)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="PC-Diagnostic"`)
			http.Error(w, "Требуется авторизация", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
