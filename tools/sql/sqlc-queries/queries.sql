-- name: GetResourceTree :many
WITH RECURSIVE tree AS (
    SELECT *
    FROM cloud_resources
    WHERE cloud_resources.id = $1

    UNION ALL

    SELECT c.*
    FROM cloud_resources c
             INNER JOIN tree t ON c.parent_resource_id = t.id
)
SELECT *
FROM tree;

-- name: GetResourceTreeByStatuses :many
WITH RECURSIVE tree AS (
    SELECT *
    FROM cloud_resources
    WHERE cloud_resources.id = $1 AND cloud_resources.status = ANY($2::int[])

    UNION ALL

    SELECT c.*
    FROM cloud_resources c
             INNER JOIN tree t ON c.parent_resource_id = t.id
)
SELECT *
FROM tree;