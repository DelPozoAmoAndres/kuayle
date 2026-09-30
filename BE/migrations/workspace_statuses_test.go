package migrations_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestWorkspaceStatusesMigrationWithExistingData guards the transition from
// per-team to per-workspace statuses (000039) and the Gitea comment columns
// (000040) against a database that already holds tenants.
//
// The bug this catches: the default status set is seeded with team_id = NULL,
// so team_id must be made nullable BEFORE the seeding INSERT runs. It only
// shows up when at least one workspace exists at migration time.
func TestWorkspaceStatusesMigrationWithExistingData(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	admin, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err)
	defer func() { _ = admin.Close() }()

	databaseName := "kuayle_migration_" + uuid.NewString()
	_, err = admin.Exec(`CREATE DATABASE "` + databaseName + `"`)
	if postgresCode(err) == "42501" {
		t.Skip("DATABASE_URL user cannot create an isolated migration database")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP DATABASE "` + databaseName + `" WITH (FORCE)`)
	})

	testURL := databaseURLWithName(t, databaseURL, databaseName)
	migrator, err := migrate.New("file://.", testURL)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = migrator.Close() })

	// Everything that predates the workspace-status migration.
	require.NoError(t, migrator.Migrate(38))
	requireMigrationVersion(t, migrator, 38)

	db, err := sql.Open("pgx", testURL)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	t.Cleanup(func() { _ = db.Close() })

	// Tenant A: two teams sharing the same status slugs (the dedup path).
	userA, workspaceA, teamA, teamB, issueA := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustExec(t, db, `INSERT INTO users (id,email,name,password_hash) VALUES ($1,'a@example.test','A','x')`, userA)
	mustExec(t, db, `INSERT INTO workspaces (id,name,slug,owner_id) VALUES ($1,'Acme','acme',$2)`, workspaceA, userA)
	mustExec(t, db, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceA, userA)
	mustExec(t, db, `INSERT INTO teams (id,workspace_id,name,key) VALUES ($1,$2,'Engineering','ENG')`, teamA, workspaceA)
	mustExec(t, db, `INSERT INTO teams (id,workspace_id,name,key) VALUES ($1,$2,'Design','DES')`, teamB, workspaceA)
	for _, teamID := range []uuid.UUID{teamA, teamB} {
		mustExec(t, db, `INSERT INTO team_statuses (team_id,name,slug,category,position,is_default)
			VALUES ($1,'Todo','todo','unstarted',0,TRUE)`, teamID)
	}
	mustExec(t, db, `INSERT INTO issues (id,workspace_id,team_id,number,identifier_text,title,creator_id,status_id)
		SELECT $1,$2,$3,1,'ENG-1','Issue',$4,id FROM team_statuses WHERE team_id=$3`, issueA, workspaceA, teamA, userA)

	// Tenant B: a workspace without any team/status at all. Seeding the default
	// set here is what used to violate the team_id NOT NULL constraint.
	userB, workspaceB := uuid.New(), uuid.New()
	mustExec(t, db, `INSERT INTO users (id,email,name,password_hash) VALUES ($1,'b@example.test','B','x')`, userB)
	mustExec(t, db, `INSERT INTO workspaces (id,name,slug,owner_id) VALUES ($1,'Lonely','lonely',$2)`, workspaceB, userB)
	mustExec(t, db, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceB, userB)

	// 000039 — workspace-level statuses.
	require.NoError(t, migrator.Steps(1))
	requireMigrationVersion(t, migrator, 39)

	var lonelyStatuses int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM team_statuses WHERE workspace_id=$1`, workspaceB).Scan(&lonelyStatuses))
	require.Equal(t, 6, lonelyStatuses, "a workspace without teams must get the default status set")

	var acmeStatuses int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM team_statuses WHERE workspace_id=$1`, workspaceA).Scan(&acmeStatuses))
	require.Equal(t, 1, acmeStatuses, "duplicate slugs inside a workspace must be merged")

	var statusID uuid.NullUUID
	require.NoError(t, db.QueryRow(`SELECT status_id FROM issues WHERE id=$1`, issueA).Scan(&statusID))
	require.True(t, statusID.Valid, "the issue must keep a status after deduplication")

	var nullCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM team_statuses WHERE workspace_id IS NULL`).Scan(&nullCount))
	require.Zero(t, nullCount)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM team_statuses WHERE team_id IS NULL AND workspace_id IS NULL`).Scan(&nullCount))
	require.Zero(t, nullCount)

	// 000040 — Gitea comment columns.
	require.NoError(t, migrator.Steps(1))
	requireMigrationVersion(t, migrator, 40)
	requireColumnExists(t, db, "users", "gitea_login")
	requireColumnExists(t, db, "comments", "gitea_comment_id")
	require.True(t, columnNullable(t, db, "comments", "user_id"), "comments.user_id must be nullable")

	// 000041 — per-user Gitea token.
	require.NoError(t, migrator.Steps(1))
	requireMigrationVersion(t, migrator, 41)
	requireColumnExists(t, db, "users", "gitea_token")

	// Roll back every migration added since 38 and restore the previous contract.
	require.NoError(t, migrator.Steps(-1))
	requireMigrationVersion(t, migrator, 40)
	requireColumnMissing(t, db, "users", "gitea_token")
	require.NoError(t, migrator.Steps(-1))
	requireMigrationVersion(t, migrator, 39)
	require.NoError(t, migrator.Steps(-1))
	requireMigrationVersion(t, migrator, 38)

	requireColumnMissing(t, db, "team_statuses", "workspace_id")
	require.False(t, columnNullable(t, db, "team_statuses", "team_id"), "team_id must be NOT NULL again")

	var teamSet int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM (
		SELECT team_id FROM team_statuses GROUP BY team_id HAVING COUNT(DISTINCT slug) = 6
	) teams_with_full_set`).Scan(&teamSet))
	require.Equal(t, 3, teamSet, "every team (including the oneless workspaces') must have the default set")

	var wrongOwner int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM issues i
		JOIN team_statuses ts ON ts.id = i.status_id
		WHERE i.team_id IS NOT NULL AND ts.team_id <> i.team_id`).Scan(&wrongOwner))
	require.Zero(t, wrongOwner, "issues must point at a status owned by their own team")

	var dangling int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM issues i
		LEFT JOIN team_statuses ts ON ts.id = i.status_id
		WHERE i.status_id IS NOT NULL AND ts.id IS NULL`).Scan(&dangling))
	require.Zero(t, dangling, "no issue may reference a deleted status")
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}

func requireColumnExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2
	)`, table, column).Scan(&exists))
	require.True(t, exists, "column %s.%s must exist", table, column)
}

func requireColumnMissing(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2
	)`, table, column).Scan(&exists))
	require.False(t, exists, "column %s.%s must be gone", table, column)
}

func columnNullable(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var isNullable string
	require.NoError(t, db.QueryRow(`SELECT is_nullable FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2`, table, column).Scan(&isNullable))
	return isNullable == "YES"
}
