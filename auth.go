package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Email string
	Pswd  string
	Ts    time.Time
}

func jwtSecretKey() []byte {
	key := os.Getenv("JWT_SECRET")
	if key == "" {
		key = "your_secret_key"
	}
	return []byte(key)
}

func generateJWT(email string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &jwt.RegisteredClaims{
		Subject:   email,
		ExpiresAt: jwt.NewNumericDate(expirationTime),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecretKey())
}

func validateJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("jwt_token")
		if err != nil || cookie == nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		claims := &jwt.RegisteredClaims{}
		token, err := jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (any, error) {
			return jwtSecretKey(), nil
		})
		if err != nil || !token.Valid {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		password := r.FormValue("password")

		if dbmap == nil {
			http.Error(w, "Database is not available", http.StatusServiceUnavailable)
			return
		}

		var user User
		itf, err := dbmap.Get(&user, email)
		if err != nil {
			log.Printf("dbmap.Get user error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if itf == nil {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		user = *(itf.(*User))
		if !checkPasswordHash(password, user.Pswd) {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		token, err := generateJWT(email)
		if err != nil {
			http.Error(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "jwt_token",
			Value:    token,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
		http.Redirect(w, r, "/openrouter", http.StatusSeeOther)
		return
	}

	tpl, err := template.ParseFiles("login.gtpl")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tpl.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
