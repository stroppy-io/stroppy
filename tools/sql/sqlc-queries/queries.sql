-- name: CountMinPinnedMessageSeq :one
SELECT min("chat_seq")::BIGINT as min_unread
FROM message
WHERE chat_id = $1
  AND pinned = true
  AND deleted_at IS NULL;

-- name: MarkMessagesAsRead :many
with updated as (
    UPDATE message
        SET read_by_ids = array_append(read_by_ids, $1::text)
        WHERE chat_id = $2 AND
              chat_seq <= $3 AND
              deleted_at IS NULL AND
              read_by_ids @> ARRAY [$1::text]
        returning *)
select *
from updated as u
limit 1000;

-- name: HasMessagesOlderThan :one
SELECT count("id") > 0
FROM message
WHERE chat_id = $1
  AND chat_seq < $2
  AND deleted_at IS NULL;

-- name: HasMessagesNewerThan :one
SELECT count("id") > 0
FROM message
WHERE chat_id = $1
  AND chat_seq > $2
  AND deleted_at IS NULL;


-- name: GetUserLastReadSeq :one
SELECT m.chat_seq
FROM message m
WHERE m.chat_id = $1
  AND $2::text = ANY (m.read_by_ids)
ORDER BY m.chat_seq DESC
LIMIT 1;

-- name: GetMaxChatSeq :one
SELECT max_seq
FROM chat
WHERE id = $1;
