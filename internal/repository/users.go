package repository

import (
	"database/sql"
	"fmt"

	"3dmodels/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(username, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	var userID int64
	err = r.db.QueryRow(
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		username, email, hash,
	).Scan(&userID)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	// Assegna ROLE_USER di default
	if err := r.AssignRoleByName(userID, "ROLE_USER"); err != nil {
		return fmt.Errorf("assign default role: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRow(
		`SELECT id, username, email, password_hash, created_at FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRow(
		`SELECT id, username, email, password_hash, created_at FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetAll() ([]models.User, error) {
	rows, err := r.db.Query(`SELECT id, username, email, password_hash, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Carica i ruoli per ogni utente
	for i := range users {
		roles, err := r.GetUserRoles(users[i].ID)
		if err != nil {
			return nil, err
		}
		users[i].Roles = roles
	}
	return users, nil
}

func (r *UserRepository) Count() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (r *UserRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE id = $1`, id)
	return err
}

func (r *UserRepository) UpdatePassword(id int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = r.db.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, hash, id)
	return err
}

// --- Ruoli ---

func (r *UserRepository) GetAllRoles() ([]models.Role, error) {
	rows, err := r.db.Query(`SELECT id, name, description, created_at FROM roles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *UserRepository) GetUserRoles(userID int64) ([]models.Role, error) {
	rows, err := r.db.Query(`
		SELECT ro.id, ro.name, ro.description, ro.created_at
		FROM roles ro
		JOIN user_roles ur ON ur.role_id = ro.id
		WHERE ur.user_id = $1
		ORDER BY ro.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *UserRepository) AssignRole(userID, roleID int64) error {
	_, err := r.db.Exec(
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, roleID,
	)
	return err
}

func (r *UserRepository) AssignRoleByName(userID int64, roleName string) error {
	var roleID int64
	err := r.db.QueryRow(`SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID)
	if err != nil {
		return fmt.Errorf("role %q not found: %w", roleName, err)
	}
	return r.AssignRole(userID, roleID)
}

func (r *UserRepository) RemoveRole(userID, roleID int64) error {
	_, err := r.db.Exec(
		`DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`,
		userID, roleID,
	)
	return err
}

func (r *UserRepository) UpdateProfile(id int64, username, email string) error {
	_, err := r.db.Exec(
		`UPDATE users SET username = $1, email = $2 WHERE id = $3`,
		username, email, id,
	)
	return err
}

func (r *UserRepository) VerifyPassword(id int64, password string) (bool, error) {
	var hash string
	err := r.db.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, id).Scan(&hash)
	if err != nil {
		return false, err
	}
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return false, nil
	}
	return true, nil
}
