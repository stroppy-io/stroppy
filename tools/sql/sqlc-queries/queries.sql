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
SELECT DISTINCT (jsonb_array_elements(c.resource_def -> 'spec' -> 'yandexCloudVm' -> 'networkInterface') ->
                 'ipAddress')::TEXT AS ip_address
FROM cloud_resources as c
WHERE status = ANY($1::int[]);
