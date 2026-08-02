// Package report exposes read-only PostgreSQL reporting tools: a workload and
// efficiency summary per employee and a per-employee task list for the current
// month. The SQL is ported verbatim from the standalone aai-mcp server. The
// tools are registered only when a database URL is configured.
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/eonyx-core/eonyx/internal/tools"
)

// emptySchema is the parameter schema for the report tools, which take no
// arguments.
var emptySchema = json.RawMessage(`{"type":"object","properties":{}}`)

// NewTools returns the report tools backed by databaseURL. The caller is
// expected to register them only when databaseURL is non-empty.
func NewTools(databaseURL string) []tools.Tool {
	return []tools.Tool{
		{
			Name:        "get_workload_summary",
			Description: "Сводная таблица загрузки и эффективности сотрудников",
			Schema:      emptySchema,
			Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
				return queryWorkload(ctx, databaseURL)
			},
		},
		{
			Name:        "get_task_details",
			Description: "Список задач сотрудников за текущий месяц (название, описание, статус) для оценки сложности и ценности работы",
			Schema:      emptySchema,
			Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
				return queryTaskDetails(ctx, databaseURL)
			},
		},
	}
}

func connect(ctx context.Context, databaseURL string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return conn, nil
}

const workloadSQL = `
WITH
  task_stats AS (
    SELECT
      t."assigneeId" AS assignee_id,
      COUNT(*)::bigint                                                           AS total_tasks,
      COUNT(*) FILTER (WHERE t.status IN ('TODO','IN_PROGRESS','PAUSED'))::bigint     AS open_tasks,
      COUNT(*) FILTER (WHERE t.status = 'DONE')::bigint                               AS done_tasks,
      (AVG(EXTRACT(EPOCH FROM (t."updatedAt" - t."createdAt")) / 86400.0)
        FILTER (WHERE t.status = 'DONE'))::float8                                     AS avg_completion_days,
      COUNT(*) FILTER (WHERE t.status = 'DONE' AND t."dueDate" IS NOT NULL)::bigint     AS done_with_due,
      COUNT(*) FILTER (
        WHERE t.status = 'DONE' AND t."dueDate" IS NOT NULL AND t."updatedAt" <= t."dueDate"
      )::bigint                                                                        AS done_on_time
    FROM "Task" t
    GROUP BY t."assigneeId"
  ),
  pause_sum AS (
    SELECT
      tpl."taskId" AS task_id,
      COALESCE(SUM(EXTRACT(EPOCH FROM (tpl."endedAt" - tpl."createdAt")) / 86400.0), 0.0) AS pause_days
    FROM "TaskPauseLog" tpl
    WHERE tpl."endedAt" IS NOT NULL
    GROUP BY tpl."taskId"
  ),
  net_time AS (
    SELECT
      t."assigneeId" AS assignee_id,
      (AVG(
        EXTRACT(EPOCH FROM (t."updatedAt" - t."createdAt")) / 86400.0
        - COALESCE(ps.pause_days, 0.0)
      ))::float8 AS avg_net_completion_days
    FROM "Task" t
    LEFT JOIN pause_sum ps ON ps.task_id = t.id
    WHERE t.status = 'DONE'
    GROUP BY t."assigneeId"
  ),
  proj_created AS (
    SELECT "userId" AS user_id, COUNT(*)::bigint AS cnt FROM "Project" GROUP BY "userId"
  ),
  proj_member AS (
    SELECT "userId" AS user_id, COUNT(*)::bigint AS cnt FROM "ProjectMember" GROUP BY "userId"
  ),
  delegated AS (
    SELECT "userId" AS user_id, COUNT(*)::bigint AS cnt
    FROM "Task"
    WHERE "assigneeId" IS NOT NULL AND "userId" != "assigneeId"
    GROUP BY "userId"
  ),
  messages AS (
    SELECT "userId" AS user_id, COUNT(*)::bigint AS cnt FROM "ProjectMessage" GROUP BY "userId"
  )
SELECT
  u.name,
  u.department,
  COALESCE(ts.open_tasks, 0)          AS open_tasks,
  COALESCE(ts.done_tasks, 0)          AS done_tasks,
  COALESCE(ts.total_tasks, 0)         AS total_tasks,
  ts.avg_completion_days,
  nt.avg_net_completion_days,
  CASE
    WHEN ts.done_with_due > 0 THEN ROUND((ts.done_on_time::numeric / ts.done_with_due) * 100, 1)::float8
    ELSE NULL
  END                                 AS pct_on_time,
  COALESCE(ts.done_with_due, 0)       AS on_time_sample,
  COALESCE(pc.cnt, 0)                 AS projects_created,
  COALESCE(pm.cnt, 0)                 AS projects_member,
  COALESCE(dl.cnt, 0)                 AS tasks_delegated,
  COALESCE(ms.cnt, 0)                 AS project_messages
FROM "User" u
LEFT JOIN task_stats ts ON ts.assignee_id = u.id
LEFT JOIN net_time nt ON nt.assignee_id = u.id
LEFT JOIN proj_created pc ON pc.user_id = u.id
LEFT JOIN proj_member pm ON pm.user_id = u.id
LEFT JOIN delegated dl ON dl.user_id = u.id
LEFT JOIN messages ms ON ms.user_id = u.id
ORDER BY ts.total_tasks DESC NULLS LAST
`

func queryWorkload(ctx context.Context, databaseURL string) (string, error) {
	conn, err := connect(ctx, databaseURL)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, workloadSQL)
	if err != nil {
		return "", fmt.Errorf("query workload: %w", err)
	}
	defer rows.Close()

	cols := []string{
		"name", "department", "open_tasks", "done_tasks", "total_tasks",
		"avg_completion_days", "avg_net_completion_days", "pct_on_time",
		"on_time_sample", "projects_created", "projects_member",
		"tasks_delegated", "project_messages",
	}
	var b csvBuilder
	b.row(cols)

	for rows.Next() {
		var (
			name                                                   string
			department                                             *string
			openTasks, doneTasks, totalTasks                       int64
			avgCompletion, avgNetCompletion, pctOnTime             *float64
			onTimeSample, projCreated, projMember, delegated, msgs int64
		)
		if err := rows.Scan(
			&name, &department, &openTasks, &doneTasks, &totalTasks,
			&avgCompletion, &avgNetCompletion, &pctOnTime,
			&onTimeSample, &projCreated, &projMember, &delegated, &msgs,
		); err != nil {
			return "", fmt.Errorf("scan workload row: %w", err)
		}
		b.row([]string{
			name,
			nullableString(department),
			strconv.FormatInt(openTasks, 10),
			strconv.FormatInt(doneTasks, 10),
			strconv.FormatInt(totalTasks, 10),
			nullableFloat(avgCompletion, 2),
			nullableFloat(avgNetCompletion, 2),
			nullableFloat(pctOnTime, 1),
			strconv.FormatInt(onTimeSample, 10),
			strconv.FormatInt(projCreated, 10),
			strconv.FormatInt(projMember, 10),
			strconv.FormatInt(delegated, 10),
			strconv.FormatInt(msgs, 10),
		})
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read workload rows: %w", err)
	}
	return b.String(), nil
}

const taskDetailsSQL = `
SELECT
  u.name AS assignee_name,
  u.department,
  t.title,
  t.description,
  t.status::text AS status
FROM "Task" t
JOIN "User" u ON u.id = t."assigneeId"
WHERE t."assigneeId" IS NOT NULL
  AND (
    t."createdAt" >= date_trunc('month', CURRENT_DATE)
    OR t."updatedAt" >= date_trunc('month', CURRENT_DATE)
  )
ORDER BY u.name, t."createdAt"
`

func queryTaskDetails(ctx context.Context, databaseURL string) (string, error) {
	conn, err := connect(ctx, databaseURL)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, taskDetailsSQL)
	if err != nil {
		return "", fmt.Errorf("query task details: %w", err)
	}
	defer rows.Close()

	var b csvBuilder
	b.row([]string{"assignee_name", "department", "title", "description", "status"})

	for rows.Next() {
		var (
			assignee, title, status string
			department, description *string
		)
		if err := rows.Scan(&assignee, &department, &title, &description, &status); err != nil {
			return "", fmt.Errorf("scan task detail row: %w", err)
		}
		b.row([]string{
			assignee,
			nullableString(department),
			title,
			nullableString(description),
			status,
		})
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read task detail rows: %w", err)
	}
	return b.String(), nil
}

// csvBuilder accumulates CSV lines, escaping fields like the original server.
type csvBuilder struct {
	lines []string
}

func (b *csvBuilder) row(fields []string) {
	escaped := make([]string, len(fields))
	for i, f := range fields {
		escaped[i] = csvEscape(f)
	}
	b.lines = append(b.lines, strings.Join(escaped, ","))
}

func (b *csvBuilder) String() string {
	return strings.Join(b.lines, "\n")
}

func csvEscape(v string) string {
	if strings.ContainsAny(v, ",\"\n") {
		return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
	}
	return v
}

func nullableString(v *string) string {
	if v == nil {
		return "NULL"
	}
	return *v
}

func nullableFloat(v *float64, prec int) string {
	if v == nil {
		return "NULL"
	}
	return strconv.FormatFloat(*v, 'f', prec, 64)
}
