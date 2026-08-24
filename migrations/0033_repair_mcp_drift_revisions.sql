-- MCP schema drift is operational status, not a published contract revision.
-- Repair rows affected by earlier drift checks without inventing phantom releases.
WITH published_mcp_releases AS (
    SELECT definition.id, max(release.version) AS published_revision
    FROM tool_definitions definition
    JOIN tool_releases release ON release.tool_definition_id = definition.id
    WHERE definition.backend_kind = 'mcp'
      AND definition.state = 'published'
    GROUP BY definition.id
)
UPDATE tool_definitions definition
SET revision = release.published_revision,
    updated_at = now()
FROM published_mcp_releases release
WHERE definition.id = release.id
  AND definition.revision <> release.published_revision;
