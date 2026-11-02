package qqcommunity

import "strings"

// Queries that depend on PostgreSQL-specific relation operators have explicit
// MySQL implementations, rather than pretending a token replacement suffices.
func mysqlRelations(q string) string {
	if strings.Contains(q, "WITH job_members AS") {
		return mysqlMemberDirectory
	}
	q = strings.ReplaceAll(q, "SELECT DISTINCT ON (group_id,member_id) * FROM recent ORDER BY group_id,member_id,created_at DESC,id DESC", `SELECT * FROM (SELECT recent.*,ROW_NUMBER() OVER(PARTITION BY group_id,member_id ORDER BY created_at DESC,id DESC) qqc_row FROM recent) latest WHERE qqc_row=1`)
	if strings.Contains(q, "generate_series(") {
		return `WITH RECURSIVE dates(d) AS (
 SELECT MIN(effective_day) FROM qqc_ranking_config_versions WHERE app_id=$1
 UNION ALL SELECT DATE_ADD(d,INTERVAL 1 DAY) FROM dates WHERE d<$2
 ) SELECT DATE_FORMAT(d,'%Y-%m-%d') FROM dates WHERE d<=$2 AND NOT EXISTS(
 SELECT 1 FROM qqc_ranking_settlements s WHERE s.app_id=$1 AND s.ranking_day=d) ORDER BY d LIMIT 7`
	}
	q = strings.ReplaceAll(q, "last_day >= $3::date-1 AND d=last_day-(n-1)", "last_day >= DATE_SUB($3,INTERVAL 1 DAY) AND DATEDIFF(last_day,d)=n-1")
	// UPDATE ... FROM has different join placement in MySQL.
	if strings.Contains(q, "UPDATE qqc_moderation_operations o SET") && strings.Contains(q, " FROM qqc_moderation_incidents i WHERE") {
		parts := strings.SplitN(q, " SET ", 2)
		rest := strings.SplitN(parts[1], " FROM qqc_moderation_incidents i WHERE ", 2)
		q = parts[0] + " JOIN qqc_moderation_incidents i ON i.id=o.incident_id SET " + rest[0] + " WHERE " + rest[1]
		q = strings.ReplaceAll(q, "THEN $1 ELSE $2", "THEN CONVERT($1 USING utf8mb4) COLLATE utf8mb4_unicode_ci ELSE CONVERT($2 USING utf8mb4) COLLATE utf8mb4_unicode_ci")
	}
	if strings.HasPrefix(q, "DELETE FROM qqc_group_messages h WHERE NOT EXISTS") {
		return `DELETE h FROM qqc_group_messages h LEFT JOIN (SELECT app_id,group_id FROM qqc_group_messages WHERE created_at>NOW()-INTERVAL '30 minutes' GROUP BY app_id,group_id) recent ON recent.app_id=h.app_id AND recent.group_id=h.group_id WHERE recent.group_id IS NULL`
	}
	if strings.HasPrefix(q, "DELETE FROM qqc_group_messages WHERE") && strings.Contains(q, "id NOT IN") {
		return `DELETE h FROM qqc_group_messages h JOIN (SELECT id,ROW_NUMBER() OVER(ORDER BY context_at DESC,id DESC) pos FROM qqc_group_messages WHERE app_id=$1 AND group_id=$2) ranked ON ranked.id=h.id WHERE ranked.pos>$3`
	}
	if strings.HasPrefix(q, "DELETE FROM qqc_group_messages WHERE") && strings.Contains(q, "AND NOT EXISTS") {
		return `DELETE h FROM qqc_group_messages h LEFT JOIN (SELECT app_id,group_id FROM qqc_group_messages WHERE created_at>NOW()-INTERVAL '30 minutes' GROUP BY app_id,group_id) recent ON recent.app_id=h.app_id AND recent.group_id=h.group_id WHERE h.app_id=$1 AND h.group_id=$2 AND recent.group_id IS NULL`
	}
	if strings.HasPrefix(q, "DELETE FROM qqc_incoming_images WHERE") && strings.Contains(q, "OFFSET $3") {
		return `DELETE h FROM qqc_incoming_images h JOIN (SELECT app_id,group_id,message_id,created_at,ROW_NUMBER() OVER(PARTITION BY app_id,group_id ORDER BY created_at DESC,message_id DESC) pos FROM qqc_incoming_images WHERE app_id=$1 AND group_id=$2) ranked ON ranked.app_id=h.app_id AND ranked.group_id=h.group_id AND ranked.message_id=h.message_id WHERE ranked.created_at<NOW()-INTERVAL '24 hours' OR ranked.pos>$3`
	}
	return q
}

const mysqlMemberDirectory = `WITH job_members AS (
 SELECT CONVERT(group_id USING utf8mb4) COLLATE utf8mb4_unicode_ci group_id,CONVERT(member_id USING utf8mb4) COLLATE utf8mb4_unicode_ci member_id,CONVERT(CASE WHEN created_at>NOW()-INTERVAL '24 hours' THEN member_nickname ELSE '' END USING utf8mb4) COLLATE utf8mb4_unicode_ci AS nickname,
 CONVERT(CASE WHEN created_at>NOW()-INTERVAL '24 hours' THEN message_preview ELSE '' END USING utf8mb4) COLLATE utf8mb4_unicode_ci AS message_preview,created_at AS seen_at
 FROM (SELECT q.*,ROW_NUMBER() OVER(PARTITION BY group_id,member_id ORDER BY created_at DESC,id DESC) qqc_row
 FROM qqc_jobs q WHERE app_id=$1 AND member_id<>'' AND event_type IN ('GROUP_AT_MESSAGE_CREATE','GROUP_MESSAGE_CREATE')
 AND ($2='' OR group_id=$2) AND ($3='' OR member_id=$3)) recent WHERE qqc_row=1
), candidates AS (
 SELECT group_id,member_id,nickname,'' AS email,message_preview,seen_at,1 AS priority FROM job_members
 UNION ALL SELECT CONVERT(group_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(member_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(nickname USING utf8mb4) COLLATE utf8mb4_unicode_ci,'',CONVERT(message_preview USING utf8mb4) COLLATE utf8mb4_unicode_ci,
 CAST(REPLACE(LEFT(seen_at,26),'T',' ') AS DATETIME(6)),1
 FROM JSON_TABLE($4,'$[*]' COLUMNS(group_id VARCHAR(191) PATH '$.group_id',member_id VARCHAR(191) PATH '$.member_id',
 nickname VARCHAR(400) PATH '$.nickname',message_preview VARCHAR(400) PATH '$.message_preview',seen_at VARCHAR(64) PATH '$.seen_at')) h
 UNION ALL SELECT CONVERT(b.group_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(b.member_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(b.nickname USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(COALESCE(u.email,'') USING utf8mb4) COLLATE utf8mb4_unicode_ci,'',NULL,2 FROM qqc_bindings b
 LEFT JOIN users u ON u.id=b.user_id AND u.deleted_at IS NULL AND u.status='active'
 WHERE b.app_id=$1 AND b.member_id<>'' AND ($2='' OR b.group_id=$2) AND ($3='' OR b.member_id=$3)
 UNION ALL SELECT CONVERT(group_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(member_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(nickname USING utf8mb4) COLLATE utf8mb4_unicode_ci,'','',NULL,3 FROM JSON_TABLE($5,'$[*]' COLUMNS(
 group_id VARCHAR(191) PATH '$.group_id',member_id VARCHAR(191) PATH '$.member_id',nickname VARCHAR(400) PATH '$.nickname')) h
 UNION ALL SELECT DISTINCT CONVERT(group_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,CONVERT(member_id USING utf8mb4) COLLATE utf8mb4_unicode_ci,'','','',NULL,4 FROM qqc_moderation_incidents
 WHERE app_id=$1 AND member_id<>'' AND ($2='' OR group_id=$2) AND ($3='' OR member_id=$3)
), members AS (
 SELECT group_id,member_id,
 COALESCE(SUBSTRING_INDEX(GROUP_CONCAT(NULLIF(nickname,'') ORDER BY priority,seen_at DESC,nickname SEPARATOR 0x1F),0x1F,1),'') nickname,
 COALESCE(MAX(email),'') email,
 COALESCE(SUBSTRING_INDEX(GROUP_CONCAT(NULLIF(message_preview,'') ORDER BY seen_at DESC,priority,message_preview SEPARATOR 0x1F),0x1F,1),'') message_preview,
 MAX(seen_at) last_seen_at FROM candidates GROUP BY group_id,member_id
), filtered AS (
 SELECT * FROM members WHERE $6='' OR POSITION(LOWER($6) IN LOWER(nickname))>0 OR POSITION(LOWER($6) IN LOWER(email))>0
 OR POSITION(LOWER($6) IN LOWER(message_preview))>0
) SELECT counted.total,paged.group_id,paged.member_id,paged.nickname,paged.email,paged.message_preview,paged.last_seen_at
 FROM (SELECT COUNT(*) total FROM filtered) counted LEFT JOIN
 (SELECT * FROM filtered ORDER BY last_seen_at DESC,group_id,member_id LIMIT $7 OFFSET $8) paged ON TRUE`
