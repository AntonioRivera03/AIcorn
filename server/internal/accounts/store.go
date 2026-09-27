package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// querier is the part of *sql.DB and *sql.Tx the store needs, so the same
// methods run inside or outside a transaction.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is the accounts repository: all SQL, no rules.
type Store struct {
	db *sql.DB
	q  querier
}

func NewStore(db *sql.DB) *Store { return &Store{db: db, q: db} }

// InTx runs fn against a Store bound to one transaction, committing when fn
// returns nil.
func (s *Store) InTx(ctx context.Context, fn func(tx *Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(&Store{db: s.db, q: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

// sqlTime formats t like SQLite's CURRENT_TIMESTAMP, so stored times compare
// correctly as text against each other and against defaults.
func sqlTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// --- accounts ---

// InsertAccount creates an account. verified is when its email address was
// proven, or nil if it hasn't been yet.
func (s *Store) InsertAccount(ctx context.Context, email, name, passwordHash string, verified *time.Time) (int64, error) {
	var verifiedAt any
	if verified != nil {
		verifiedAt = sqlTime(*verified)
	}
	var id int64
	err := s.q.QueryRowContext(ctx,
		`INSERT INTO account(email, name, passwordHash, timeEmailVerified) VALUES(?, ?, ?, ?) RETURNING id`,
		email, name, passwordHash, verifiedAt).Scan(&id)
	return id, err
}

func (s *Store) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := s.q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account WHERE email = ?)`, email).Scan(&exists)
	return exists, err
}

const accountColumns = `id, email, name, COALESCE(usage, ''), timeEmailVerified IS NOT NULL`

func scanAccount(row interface{ Scan(...any) error }, extra ...any) (Account, error) {
	var a Account
	err := row.Scan(append([]any{&a.ID, &a.Email, &a.Name, &a.Usage, &a.EmailVerified}, extra...)...)
	return a, err
}

// AccountByEmail returns the account and its password hash.
func (s *Store) AccountByEmail(ctx context.Context, email string) (Account, string, error) {
	var hash string
	a, err := scanAccount(s.q.QueryRowContext(ctx,
		`SELECT `+accountColumns+`, passwordHash FROM account WHERE email = ?`, email), &hash)
	return a, hash, notFound(err)
}

func (s *Store) AccountByID(ctx context.Context, id int64) (Account, error) {
	a, err := scanAccount(s.q.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM account WHERE id = ?`, id))
	return a, notFound(err)
}

func (s *Store) SetUsage(ctx context.Context, accountID int64, usage Usage) error {
	_, err := s.q.ExecContext(ctx, `UPDATE account SET usage = ? WHERE id = ?`, usage, accountID)
	return err
}

func (s *Store) SetName(ctx context.Context, accountID int64, name string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE account SET name = ? WHERE id = ?`, name, accountID)
	return err
}

func (s *Store) PasswordHash(ctx context.Context, accountID int64) (string, error) {
	var hash string
	err := s.q.QueryRowContext(ctx, `SELECT passwordHash FROM account WHERE id = ?`, accountID).Scan(&hash)
	return hash, notFound(err)
}

func (s *Store) SetPasswordHash(ctx context.Context, accountID int64, hash string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE account SET passwordHash = ? WHERE id = ?`, hash, accountID)
	return err
}

// MarkEmailVerified records that the account proved its email address. An
// already-verified account keeps its original time.
func (s *Store) MarkEmailVerified(ctx context.Context, accountID int64, now time.Time) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE account SET timeEmailVerified = COALESCE(timeEmailVerified, ?) WHERE id = ?`,
		sqlTime(now), accountID)
	return err
}

// --- sessions ---

func (s *Store) InsertSession(ctx context.Context, tokenHash string, accountID int64, expires time.Time) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO session(tokenHash, account, timeExpires) VALUES(?, ?, ?)`,
		tokenHash, accountID, sqlTime(expires))
	return err
}

// SessionAccount returns the account behind a live session and when the
// session expires. Expired sessions read as not found.
func (s *Store) SessionAccount(ctx context.Context, tokenHash string, now time.Time) (Account, time.Time, error) {
	var expires time.Time
	a, err := scanAccount(s.q.QueryRowContext(ctx,
		`SELECT a.id, a.email, a.name, COALESCE(a.usage, ''), a.timeEmailVerified IS NOT NULL, s.timeExpires
		   FROM session s JOIN account a ON a.id = s.account
		  WHERE s.tokenHash = ? AND s.timeExpires > ?`,
		tokenHash, sqlTime(now)), &expires)
	return a, expires, notFound(err)
}

func (s *Store) ExtendSession(ctx context.Context, tokenHash string, expires time.Time) error {
	_, err := s.q.ExecContext(ctx, `UPDATE session SET timeExpires = ? WHERE tokenHash = ?`, sqlTime(expires), tokenHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM session WHERE tokenHash = ?`, tokenHash)
	return err
}

// DeleteOtherSessions signs the account out everywhere except the session
// whose token hashes to keepTokenHash (pass "" to sign out everywhere).
func (s *Store) DeleteOtherSessions(ctx context.Context, accountID int64, keepTokenHash string) error {
	_, err := s.q.ExecContext(ctx,
		`DELETE FROM session WHERE account = ? AND tokenHash <> ?`, accountID, keepTokenHash)
	return err
}

// DeleteExpired removes expired sessions and one-time tokens.
func (s *Store) DeleteExpired(ctx context.Context, now time.Time) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM session WHERE timeExpires <= ?`, sqlTime(now)); err != nil {
		return err
	}
	_, err := s.q.ExecContext(ctx, `DELETE FROM account_token WHERE timeExpires <= ?`, sqlTime(now))
	return err
}

// --- one-time tokens ---

func (s *Store) InsertToken(ctx context.Context, tokenHash string, accountID int64, purpose tokenPurpose, expires time.Time) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO account_token(tokenHash, account, purpose, timeExpires) VALUES(?, ?, ?, ?)`,
		tokenHash, accountID, purpose, sqlTime(expires))
	return err
}

func (s *Store) DeleteTokens(ctx context.Context, accountID int64, purpose tokenPurpose) error {
	_, err := s.q.ExecContext(ctx,
		`DELETE FROM account_token WHERE account = ? AND purpose = ?`, accountID, purpose)
	return err
}

// AccountForToken returns the account a live token of the given purpose
// belongs to. Expired tokens read as not found.
func (s *Store) AccountForToken(ctx context.Context, tokenHash string, purpose tokenPurpose, now time.Time) (Account, error) {
	a, err := scanAccount(s.q.QueryRowContext(ctx,
		`SELECT a.id, a.email, a.name, COALESCE(a.usage, ''), a.timeEmailVerified IS NOT NULL
		   FROM account_token t JOIN account a ON a.id = t.account
		  WHERE t.tokenHash = ? AND t.purpose = ? AND t.timeExpires > ?`,
		tokenHash, purpose, sqlTime(now)))
	return a, notFound(err)
}

// --- workspaces & memberships ---

func (s *Store) InsertWorkspace(ctx context.Context, kind WorkspaceKind, name string, personalAccount *int64) (int64, error) {
	var id int64
	err := s.q.QueryRowContext(ctx,
		`INSERT INTO workspace(kind, name, personalAccount) VALUES(?, ?, ?) RETURNING id`,
		kind, name, personalAccount).Scan(&id)
	return id, err
}

func (s *Store) InsertMembership(ctx context.Context, workspaceID, accountID int64, role Role) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO membership(workspace, account, role) VALUES(?, ?, ?)`,
		workspaceID, accountID, role)
	return err
}

// WorkspacesForAccount lists the account's workspaces, personal first, then
// organizations by name.
func (s *Store) WorkspacesForAccount(ctx context.Context, accountID int64) ([]Workspace, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT w.id, w.kind, w.name, m.role
		   FROM membership m JOIN workspace w ON w.id = m.workspace
		  WHERE m.account = ?
		  ORDER BY w.kind = 'personal' DESC, w.name COLLATE NOCASE, w.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := []Workspace{}
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Kind, &w.Name, &w.Role); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, w)
	}
	return workspaces, rows.Err()
}

// WorkspaceForMember returns the workspace as seen by accountID, or
// ErrNotFound when they aren't a member.
func (s *Store) WorkspaceForMember(ctx context.Context, workspaceID, accountID int64) (Workspace, error) {
	var w Workspace
	err := s.q.QueryRowContext(ctx,
		`SELECT w.id, w.kind, w.name, m.role
		   FROM membership m JOIN workspace w ON w.id = m.workspace
		  WHERE m.workspace = ? AND m.account = ?`, workspaceID, accountID).
		Scan(&w.ID, &w.Kind, &w.Name, &w.Role)
	return w, notFound(err)
}

func (s *Store) RenameWorkspace(ctx context.Context, workspaceID int64, name string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE workspace SET name = ? WHERE id = ?`, name, workspaceID)
	return err
}

func (s *Store) AllWorkspaceIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id FROM workspace ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) Members(ctx context.Context, workspaceID int64) ([]Member, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT a.id, a.name, a.email, m.role, m.timeCreated
		   FROM membership m JOIN account a ON a.id = m.account
		  WHERE m.workspace = ?
		  ORDER BY a.name COLLATE NOCASE, a.id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.AccountID, &m.Name, &m.Email, &m.Role, &m.TimeJoined); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *Store) CountOwners(ctx context.Context, workspaceID int64) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM membership WHERE workspace = ? AND role = 'owner'`, workspaceID).Scan(&n)
	return n, err
}

func (s *Store) UpdateMemberRole(ctx context.Context, workspaceID, accountID int64, role Role) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE membership SET role = ? WHERE workspace = ? AND account = ?`, role, workspaceID, accountID)
	return requireRow(res, err)
}

func (s *Store) DeleteMembership(ctx context.Context, workspaceID, accountID int64) error {
	res, err := s.q.ExecContext(ctx,
		`DELETE FROM membership WHERE workspace = ? AND account = ?`, workspaceID, accountID)
	return requireRow(res, err)
}

func (s *Store) IsMemberByEmail(ctx context.Context, workspaceID int64, email string) (bool, error) {
	var exists bool
	err := s.q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM membership m JOIN account a ON a.id = m.account
		                WHERE m.workspace = ? AND a.email = ?)`, workspaceID, email).Scan(&exists)
	return exists, err
}

// --- invites ---

func (s *Store) RevokePendingInvites(ctx context.Context, workspaceID int64, email string, now time.Time) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE invite SET timeRevoked = ?
		  WHERE workspace = ? AND email = ? AND timeAccepted IS NULL AND timeRevoked IS NULL`,
		sqlTime(now), workspaceID, email)
	return err
}

func (s *Store) InsertInvite(ctx context.Context, workspaceID int64, email string, role Role, codeHash string, invitedBy int64, expires time.Time) (Invite, error) {
	inv := Invite{Email: email, Role: role, TimeExpires: expires}
	err := s.q.QueryRowContext(ctx,
		`INSERT INTO invite(workspace, email, role, codeHash, invitedBy, timeExpires)
		 VALUES(?, ?, ?, ?, ?, ?) RETURNING id, timeCreated`,
		workspaceID, email, role, codeHash, invitedBy, sqlTime(expires)).Scan(&inv.ID, &inv.TimeCreated)
	return inv, err
}

// PendingInvites lists invites that can still be accepted.
func (s *Store) PendingInvites(ctx context.Context, workspaceID int64, now time.Time) ([]Invite, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT i.id, i.email, i.role, COALESCE(a.name, ''), i.timeCreated, i.timeExpires
		   FROM invite i LEFT JOIN account a ON a.id = i.invitedBy
		  WHERE i.workspace = ? AND i.timeAccepted IS NULL AND i.timeRevoked IS NULL AND i.timeExpires > ?
		  ORDER BY i.timeCreated DESC, i.id DESC`, workspaceID, sqlTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := []Invite{}
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.InvitedBy, &inv.TimeCreated, &inv.TimeExpires); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

func (s *Store) RevokeInvite(ctx context.Context, workspaceID, inviteID int64, now time.Time) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE invite SET timeRevoked = ?
		  WHERE id = ? AND workspace = ? AND timeAccepted IS NULL AND timeRevoked IS NULL`,
		sqlTime(now), inviteID, workspaceID)
	return requireRow(res, err)
}

// pendingInvite is an invite that can still be accepted, joined with the
// names shown to the invitee.
type pendingInvite struct {
	ID            int64
	WorkspaceID   int64
	WorkspaceName string
	Email         string
	Role          Role
	InvitedBy     string
}

func (s *Store) PendingInviteByCodeHash(ctx context.Context, codeHash string, now time.Time) (pendingInvite, error) {
	var inv pendingInvite
	err := s.q.QueryRowContext(ctx,
		`SELECT i.id, w.id, w.name, i.email, i.role, COALESCE(a.name, '')
		   FROM invite i
		   JOIN workspace w ON w.id = i.workspace
		   LEFT JOIN account a ON a.id = i.invitedBy
		  WHERE i.codeHash = ? AND i.timeAccepted IS NULL AND i.timeRevoked IS NULL AND i.timeExpires > ?`,
		codeHash, sqlTime(now)).
		Scan(&inv.ID, &inv.WorkspaceID, &inv.WorkspaceName, &inv.Email, &inv.Role, &inv.InvitedBy)
	return inv, notFound(err)
}

// MarkInviteAccepted consumes the invite. It affects no row if another
// request consumed it first, which is what makes an invite single-use.
func (s *Store) MarkInviteAccepted(ctx context.Context, inviteID, accountID int64, now time.Time) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE invite SET timeAccepted = ?, acceptedBy = ?
		  WHERE id = ? AND timeAccepted IS NULL AND timeRevoked IS NULL`,
		sqlTime(now), accountID, inviteID)
	return requireRow(res, err)
}

func requireRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
