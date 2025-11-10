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


-- name: GetAllResourcesIps :many
SELECT DISTINCT (ni ->> 'ip_address')::TEXT AS ip_address
FROM cloud_resources
         CROSS JOIN LATERAL jsonb_array_elements(
        resource_def -> 'spec' -> 'yandexCloudVm' -> 'networkInterface'
                            ) AS ni
WHERE ni ? 'ip_address'
  AND ni ->> 'ip_address' IS NOT NULL
  AND ni ->> 'ip_address' <> ''
AND status = ANY($1::int[]);