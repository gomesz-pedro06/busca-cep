package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Clientes struct {
	ID           int       `json:"id"`
	Nome         string    `json:"nome"`
	Email        string    `json:"email"`
	Telefone     string    `json:"telefone"`
	CriadoEm     time.Time `json:"criado_em"`
	AtualizadoEm time.Time `json:"atualizado_em"`
}

var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("postgres", "postgres://pedro:123456@postgres/banco?sslmode=disable")
	if err != nil {
		panic(err)
	}
	if err := db.Ping(); err != nil {
		panic(err)
	}
	fmt.Println("Você está conectado")
}

var emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)

func Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	c := Clientes{}
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, "Erro ao decodificar JSON", http.StatusBadRequest)
		return
	}

	c.Nome = strings.TrimSpace(c.Nome)
	if c.Nome == "" {
		http.Error(w, "O campo 'nome' é obrigatório.", http.StatusUnprocessableEntity)
		return
	}

	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	if c.Email == "" {
		http.Error(w, "O campo 'email' é obrigatório.", http.StatusUnprocessableEntity)
		return
	}
	if !emailRegex.MatchString(c.Email) {
		http.Error(w, "O formato do 'email' é inválido.", http.StatusUnprocessableEntity)
		return
	}

	c.Telefone = strings.TrimSpace(c.Telefone)
	if c.Telefone == "" {
		http.Error(w, "O campo 'telefone' é obrigatório.", http.StatusUnprocessableEntity)
		return
	}
	apenasNumeros := regexp.MustCompile(`[^\d]`).ReplaceAllString(c.Telefone, "")
	if len(apenasNumeros) != 11 {
		http.Error(w, "O 'telefone' deve conter 11 dígitos numéricos.", http.StatusUnprocessableEntity)
		return
	}

	var existeID int

	err = db.QueryRow("SELECT id FROM clientes WHERE email = $1", c.Email).Scan(&existeID)
	if err == nil {
		http.Error(w, "Este e-mail já está cadastrado.", http.StatusUnprocessableEntity)
		return
	} else if err != sql.ErrNoRows {
		http.Error(w, "Erro ao verificar e-mail", http.StatusInternalServerError)
		return
	}

	err = db.QueryRow("SELECT id FROM clientes WHERE telefone = $1", c.Telefone).Scan(&existeID)
	if err == nil {
		http.Error(w, "Este telefone já está cadastrado.", http.StatusUnprocessableEntity)
		return
	} else if err != sql.ErrNoRows {
		http.Error(w, "Erro ao verificar telefone", http.StatusInternalServerError)
		return
	}

	query := `INSERT INTO clientes (nome, email, telefone) VALUES ($1, $2, $3) RETURNING id, criado_em, atualizado_em`
	err = db.QueryRow(query, c.Nome, c.Email, c.Telefone).Scan(&c.ID, &c.CriadoEm, &c.AtualizadoEm)
	if err != nil {
		http.Error(w, "Erro ao criar cliente", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

func Read(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query("SELECT id, nome, email, telefone, criado_em, atualizado_em FROM clientes")
	if err != nil {
		http.Error(w, "Erro ao buscar clientes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	data := make([]Clientes, 0)

	for rows.Next() {
		var c Clientes
		err := rows.Scan(&c.ID, &c.Nome, &c.Email, &c.Telefone, &c.CriadoEm, &c.AtualizadoEm)
		if err != nil {
			http.Error(w, "Erro ao ler cliente", http.StatusInternalServerError)
			return
		}
		data = append(data, c)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Erro ao iterar sobre os clientes", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	c := Clientes{}
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, "Erro ao decodificar JSON", http.StatusBadRequest)
		return
	}

	query := `UPDATE clientes SET nome = $1, email = $2, telefone = $3, atualizado_em = NOW() WHERE id = $4`
	_, err = db.Exec(query, c.Nome, c.Email, c.Telefone, id)
	if err != nil {
		http.Error(w, "Erro ao atualizar cliente", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID do cliente não fornecido", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Erro ao iniciar exclusão", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec("DELETE FROM clientes WHERE id = $1", id)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Erro ao deletar cliente", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec("UPDATE clientes SET id = -id")
	if err != nil {
		tx.Rollback()
		http.Error(w, "Erro ao reorganizar IDs dos clientes", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`
		WITH numerados AS (
			SELECT ctid, ROW_NUMBER() OVER (ORDER BY id DESC) AS novo_id
			FROM clientes
		)
		UPDATE clientes c
		SET id = n.novo_id
		FROM numerados n
		WHERE c.ctid = n.ctid`)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Erro ao reorganizar IDs dos clientes", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`
		SELECT setval(
			pg_get_serial_sequence('clientes', 'id'),
			COALESCE((SELECT MAX(id) FROM clientes), 1),
			(SELECT COUNT(*) > 0 FROM clientes)
		)`)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Erro ao sincronizar ID dos clientes", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "Erro ao confirmar exclusão", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/clientes/criar", Create)
	http.HandleFunc("/clientes/listar", Read)
	http.HandleFunc("/clientes/atualizar", Update)
	http.HandleFunc("/clientes/deletar", Delete)
	http.ListenAndServe(":8080", nil)
}
