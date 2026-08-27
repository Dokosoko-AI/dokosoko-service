package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type recipeQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

const analysisSelect = `SELECT id::text,organisation_id::text,product_id::text,schema_version,state,generated_by,evidence,plan,unknowns,error_code,revision,created_at,completed_at FROM integration_analyses`

func scanIntegrationAnalysis(row pgx.Row) (model.IntegrationAnalysis, error) {
	var value model.IntegrationAnalysis
	var evidence, plan, unknowns []byte
	if err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.SchemaVersion, &value.State, &value.GeneratedBy, &evidence, &plan, &unknowns, &value.ErrorCode, &value.Revision, &value.CreatedAt, &value.CompletedAt); err != nil {
		return value, databaseError(err)
	}
	if err := json.Unmarshal(evidence, &value.Evidence); err != nil {
		return value, err
	}
	if err := json.Unmarshal(plan, &value.Plan); err != nil {
		return value, err
	}
	if err := json.Unmarshal(unknowns, &value.Unknowns); err != nil {
		return value, err
	}
	return value, nil
}

func (p *Postgres) IntegrationAnalyses(ctx context.Context, productID string) ([]model.IntegrationAnalysis, error) {
	rows, err := p.pool.Query(ctx, analysisSelect+` WHERE product_id=$1 ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.IntegrationAnalysis, 0)
	for rows.Next() {
		value, err := scanIntegrationAnalysis(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) IntegrationAnalysis(ctx context.Context, productID, id string) (model.IntegrationAnalysis, error) {
	return scanIntegrationAnalysis(p.pool.QueryRow(ctx, analysisSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) SaveIntegrationAnalysis(ctx context.Context, value model.IntegrationAnalysis, expected int64) (model.IntegrationAnalysis, error) {
	evidence, _ := json.Marshal(value.Evidence)
	plan, _ := json.Marshal(value.Plan)
	unknowns, _ := json.Marshal(value.Unknowns)
	if expected == 0 {
		return scanIntegrationAnalysis(p.pool.QueryRow(ctx, `INSERT INTO integration_analyses(id,organisation_id,product_id,schema_version,state,generated_by,evidence,plan,unknowns,error_code,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text,organisation_id::text,product_id::text,schema_version,state,generated_by,evidence,plan,unknowns,error_code,revision,created_at,completed_at`, value.ID, value.OrganisationID, value.ProductID, value.SchemaVersion, value.State, value.GeneratedBy, evidence, plan, unknowns, value.ErrorCode, value.CompletedAt))
	}
	updated, err := scanIntegrationAnalysis(p.pool.QueryRow(ctx, `UPDATE integration_analyses SET state=$3,generated_by=$4,evidence=$5,plan=$6,unknowns=$7,error_code=$8,completed_at=$9,revision=revision+1 WHERE product_id=$1 AND id=$2 AND revision=$10 RETURNING id::text,organisation_id::text,product_id::text,schema_version,state,generated_by,evidence,plan,unknowns,error_code,revision,created_at,completed_at`, value.ProductID, value.ID, value.State, value.GeneratedBy, evidence, plan, unknowns, value.ErrorCode, value.CompletedAt, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IntegrationAnalysis(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.IntegrationAnalysis{}, ErrConflict
		}
	}
	return updated, err
}

const recipeSelect = `SELECT id::text,organisation_id::text,product_id::text,coalesce(integration_id::text,''),coalesce(analysis_id::text,''),contract_version,slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,coalesce(current_revision_id::text,''),stable_uri,approved_by,approved_at,published_at,revision,created_at,updated_at FROM recipes`

func scanRecipe(row pgx.Row) (model.Recipe, error) {
	var value model.Recipe
	var dependencies []byte
	if err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.IntegrationID, &value.AnalysisID, &value.ContractVersion, &value.Slug, &value.Title, &value.Outcome, &value.Audience, &value.State, &value.Generated, &value.NeedsAttention, &value.Visibility, &dependencies, &value.CurrentRevisionID, &value.StableURI, &value.ApprovedBy, &value.ApprovedAt, &value.PublishedAt, &value.Revision, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return value, databaseError(err)
	}
	if err := json.Unmarshal(dependencies, &value.Dependencies); err != nil {
		return value, err
	}
	return value, nil
}

func (p *Postgres) hydrateRecipe(ctx context.Context, value model.Recipe) (model.Recipe, error) {
	var err error
	value.APIAttachments, err = recipeAPIAttachments(ctx, p.pool, value.ID)
	if err != nil {
		return value, err
	}
	if value.CurrentRevisionID == "" {
		return value, nil
	}
	revision, err := scanRecipeRevision(p.pool.QueryRow(ctx, recipeRevisionSelect+` WHERE recipe_id=$1 AND id=$2`, value.ID, value.CurrentRevisionID))
	if err != nil {
		return value, err
	}
	revision, err = hydrateRecipeRevision(ctx, p.pool, revision)
	if err != nil {
		return value, err
	}
	value.CurrentRevision = &revision
	return value, nil
}

func recipeAPIAttachments(ctx context.Context, query recipeQuerier, recipeID string) ([]model.RecipeAPIAttachment, error) {
	rows, err := query.Query(ctx, `SELECT integration_id::text FROM recipe_api_attachments WHERE recipe_id=$1 ORDER BY integration_id`, recipeID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.RecipeAPIAttachment, 0)
	for rows.Next() {
		var value model.RecipeAPIAttachment
		if err := rows.Scan(&value.IntegrationID); err != nil {
			return nil, databaseError(err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func hydrateRecipeRevision(ctx context.Context, query recipeQuerier, value model.RecipeRevision) (model.RecipeRevision, error) {
	rows, err := query.Query(ctx, `SELECT integration_id::text,integration_revision_id::text,integration_manifest_hash FROM recipe_revision_api_bindings WHERE recipe_revision_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return value, databaseError(err)
	}
	defer rows.Close()
	value.APIBindings = make([]model.RecipeAPIBinding, 0)
	for rows.Next() {
		var binding model.RecipeAPIBinding
		if err := rows.Scan(&binding.IntegrationID, &binding.IntegrationRevisionID, &binding.IntegrationManifestHash); err != nil {
			return value, databaseError(err)
		}
		value.APIBindings = append(value.APIBindings, binding)
	}
	return value, rows.Err()
}

func replaceRecipeAPIAttachments(ctx context.Context, query recipeQuerier, recipe model.Recipe, actorID string) error {
	if _, err := query.Exec(ctx, `DELETE FROM recipe_api_attachments WHERE recipe_id=$1`, recipe.ID); err != nil {
		return databaseError(err)
	}
	for _, attachment := range recipe.APIAttachments {
		if _, err := query.Exec(ctx, `INSERT INTO recipe_api_attachments(recipe_id,deployment_id,integration_id,created_by) VALUES($1,$2,$3,$4)`, recipe.ID, recipe.ProductID, attachment.IntegrationID, actorID); err != nil {
			return databaseError(err)
		}
	}
	return nil
}

func insertRecipeAPIBindings(ctx context.Context, query recipeQuerier, recipe model.Recipe, revision model.RecipeRevision) error {
	for _, binding := range revision.APIBindings {
		if _, err := query.Exec(ctx, `INSERT INTO recipe_revision_api_bindings(recipe_revision_id,recipe_id,deployment_id,integration_id,integration_revision_id,integration_manifest_hash) VALUES($1,$2,$3,$4,$5,$6)`, revision.ID, recipe.ID, recipe.ProductID, binding.IntegrationID, binding.IntegrationRevisionID, binding.IntegrationManifestHash); err != nil {
			return databaseError(err)
		}
	}
	return nil
}

func (p *Postgres) Recipes(ctx context.Context, productID string) ([]model.Recipe, error) {
	rows, err := p.pool.Query(ctx, recipeSelect+` WHERE product_id=$1 ORDER BY updated_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.Recipe, 0)
	for rows.Next() {
		value, err := scanRecipe(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range values {
		values[index], err = p.hydrateRecipe(ctx, values[index])
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (p *Postgres) Recipe(ctx context.Context, productID, id string) (model.Recipe, error) {
	value, err := scanRecipe(p.pool.QueryRow(ctx, recipeSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
	if err != nil {
		return value, err
	}
	return p.hydrateRecipe(ctx, value)
}

func (p *Postgres) RecipeBySlug(ctx context.Context, productID, slug string) (model.Recipe, error) {
	value, err := scanRecipe(p.pool.QueryRow(ctx, recipeSelect+` WHERE product_id=$1 AND slug=$2`, productID, slug))
	if err != nil {
		return value, err
	}
	return p.hydrateRecipe(ctx, value)
}

func (p *Postgres) DeleteRecipe(ctx context.Context, productID, recipeID string, mutation RecipeMutation) error {
	if err := validateRecipeMutation(mutation); err != nil {
		return err
	}
	if mutation.Audit == nil {
		return errors.New("recipe deletion requires an audit event")
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	stored, err := scanRecipe(tx.QueryRow(ctx, recipeSelect+` WHERE product_id=$1 AND id=$2 FOR UPDATE`, productID, recipeID))
	if err != nil {
		return err
	}
	if stored.Revision != mutation.ExpectedRevision || !recipeDeletionAllowed(stored) {
		return ErrConflict
	}
	prior, current, outcome, err := prepareRecipeAudit(stored, mutation.Audit, "recipe.deleted")
	if err != nil {
		return err
	}
	if err := insertRecipeAudit(ctx, tx, *mutation.Audit, prior, current, outcome); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM recipes WHERE product_id=$1 AND id=$2 AND revision=$3`, productID, recipeID, mutation.ExpectedRevision)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	if stored.State == "published" {
		if err := bumpProductCatalogRevisionTx(ctx, tx, productID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return databaseError(err)
	}
	return nil
}

func updateRecipeRow(ctx context.Context, query pgxRowQuerier, value model.Recipe, dependencies []byte, expected int64) (model.Recipe, error) {
	return scanRecipe(query.QueryRow(ctx, `UPDATE recipes SET integration_id=nullif($3,'')::uuid,analysis_id=nullif($4,'')::uuid,contract_version=coalesce(nullif($5,''),contract_version),title=$6,outcome=$7,audience=$8,state=$9,needs_attention=$10,visibility=$11,dependencies=$12,current_revision_id=nullif($13,'')::uuid,approved_by=$14,approved_at=$15,published_at=$16,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$17 RETURNING id::text,organisation_id::text,product_id::text,coalesce(integration_id::text,''),coalesce(analysis_id::text,''),contract_version,slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,coalesce(current_revision_id::text,''),stable_uri,approved_by,approved_at,published_at,revision,created_at,updated_at`, value.ProductID, value.ID, value.IntegrationID, value.AnalysisID, value.ContractVersion, value.Title, value.Outcome, value.Audience, value.State, value.NeedsAttention, value.Visibility, dependencies, value.CurrentRevisionID, value.ApprovedBy, value.ApprovedAt, value.PublishedAt, expected))
}

func createRecipeRow(ctx context.Context, query pgxRowQuerier, value model.Recipe, dependencies []byte) (model.Recipe, error) {
	return scanRecipe(query.QueryRow(ctx, `INSERT INTO recipes(id,organisation_id,product_id,integration_id,analysis_id,contract_version,slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,current_revision_id,stable_uri,approved_by,approved_at,published_at) VALUES($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,coalesce(nullif($6,''),'legacy-mcp-v1'),$7,$8,$9,$10,$11,$12,$13,$14,$15,nullif($16,'')::uuid,$17,$18,$19,$20) RETURNING id::text,organisation_id::text,product_id::text,coalesce(integration_id::text,''),coalesce(analysis_id::text,''),contract_version,slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,coalesce(current_revision_id::text,''),stable_uri,approved_by,approved_at,published_at,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.IntegrationID, value.AnalysisID, value.ContractVersion, value.Slug, value.Title, value.Outcome, value.Audience, value.State, value.Generated, value.NeedsAttention, value.Visibility, dependencies, value.CurrentRevisionID, value.StableURI, value.ApprovedBy, value.ApprovedAt, value.PublishedAt))
}

func (p *Postgres) SaveRecipeTransition(ctx context.Context, recipe model.Recipe, mutation RecipeMutation) (model.Recipe, error) {
	var err error
	recipe, err = prepareRecipeRecord(recipe)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := validateRecipeMutation(mutation); err != nil {
		return model.Recipe{}, err
	}
	if mutation.Audit == nil {
		return model.Recipe{}, errors.New("recipe transitions require an audit event")
	}
	prior, current, auditOutcome, err := prepareRecipeAudit(recipe, mutation.Audit, recipeTransitionAuditActions(model.Recipe{}, recipe)...)
	if err != nil {
		return model.Recipe{}, err
	}
	dependencies, _ := json.Marshal(recipe.Dependencies)

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Recipe{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRecipeCatalog(ctx, tx, recipe.ProductID, mutation.ExpectedCatalogRevision); err != nil {
		return model.Recipe{}, err
	}
	stored, err := scanRecipe(tx.QueryRow(ctx, recipeSelect+` WHERE product_id=$1 AND id=$2 FOR UPDATE`, recipe.ProductID, recipe.ID))
	if err != nil {
		return model.Recipe{}, err
	}
	stored.APIAttachments, err = recipeAPIAttachments(ctx, tx, stored.ID)
	if err != nil {
		return model.Recipe{}, err
	}
	if stored.Revision != mutation.ExpectedRevision {
		return model.Recipe{}, ErrConflict
	}
	if err := validateRecipeTransition(stored, recipe); err != nil {
		return model.Recipe{}, err
	}
	saved, err := updateRecipeRow(ctx, tx, recipe, dependencies, mutation.ExpectedRevision)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return model.Recipe{}, ErrConflict
		}
		return model.Recipe{}, err
	}
	if recipeTransitionBumpsCatalog(stored, saved) {
		if err := bumpDeploymentCatalog(ctx, tx, saved.ProductID); err != nil {
			return model.Recipe{}, err
		}
	}
	if mutation.Audit != nil {
		if err := insertRecipeAudit(ctx, tx, *mutation.Audit, prior, current, auditOutcome); err != nil {
			return model.Recipe{}, err
		}
	}
	if saved.CurrentRevisionID != "" {
		revision, err := scanRecipeRevision(tx.QueryRow(ctx, recipeRevisionSelect+` WHERE recipe_id=$1 AND id=$2`, saved.ID, saved.CurrentRevisionID))
		if err != nil {
			return model.Recipe{}, err
		}
		revision, err = hydrateRecipeRevision(ctx, tx, revision)
		if err != nil {
			return model.Recipe{}, err
		}
		saved.CurrentRevision = &revision
	}
	saved.APIAttachments = append([]model.RecipeAPIAttachment(nil), recipe.APIAttachments...)
	if err := tx.Commit(ctx); err != nil {
		return model.Recipe{}, databaseError(err)
	}
	return saved, nil
}

const recipeRevisionSelect = `SELECT id::text,recipe_id::text,revision,spec_version,spec,markdown,reference_items,validation,review,generated_by,model,coalesce(integration_revision_id::text,''),integration_manifest_hash,prompt_version,prompt_hash,created_by,created_at FROM recipe_revisions`

func scanRecipeRevision(row pgx.Row) (model.RecipeRevision, error) {
	var value model.RecipeRevision
	var spec, references, validation []byte
	if err := row.Scan(&value.ID, &value.RecipeID, &value.Revision, &value.SpecVersion, &spec, &value.Markdown, &references, &validation, &value.Review, &value.GeneratedBy, &value.Model, &value.IntegrationRevisionID, &value.IntegrationManifestHash, &value.PromptVersion, &value.PromptHash, &value.CreatedBy, &value.CreatedAt); err != nil {
		return value, databaseError(err)
	}
	value.Spec = append(json.RawMessage(nil), spec...)
	if err := json.Unmarshal(references, &value.References); err != nil {
		return value, err
	}
	if err := json.Unmarshal(validation, &value.Validation); err != nil {
		return value, err
	}
	return value, nil
}

func (p *Postgres) RecipeRevisions(ctx context.Context, recipeID string) ([]model.RecipeRevision, error) {
	rows, err := p.pool.Query(ctx, recipeRevisionSelect+` WHERE recipe_id=$1 ORDER BY revision DESC`, recipeID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.RecipeRevision, 0)
	for rows.Next() {
		value, err := scanRecipeRevision(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range values {
		values[index], err = hydrateRecipeRevision(ctx, p.pool, values[index])
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func createRecipeRevisionRow(ctx context.Context, query pgxRowQuerier, value model.RecipeRevision) (model.RecipeRevision, error) {
	references, _ := json.Marshal(value.References)
	validation, _ := json.Marshal(value.Validation)
	return scanRecipeRevision(query.QueryRow(ctx, `INSERT INTO recipe_revisions(id,recipe_id,revision,spec_version,spec,markdown,reference_items,validation,review,generated_by,model,integration_revision_id,integration_manifest_hash,prompt_version,prompt_hash,created_by) VALUES($1,$2,coalesce(nullif($3,0),(SELECT coalesce(max(revision),0)+1 FROM recipe_revisions WHERE recipe_id=$2)),coalesce(nullif($4,0),1),$5,$6,$7,$8,$9,$10,$11,nullif($12,'')::uuid,$13,$14,$15,$16) RETURNING id::text,recipe_id::text,revision,spec_version,spec,markdown,reference_items,validation,review,generated_by,model,coalesce(integration_revision_id::text,''),integration_manifest_hash,prompt_version,prompt_hash,created_by,created_at`, value.ID, value.RecipeID, value.Revision, value.SpecVersion, value.Spec, value.Markdown, references, validation, value.Review, value.GeneratedBy, value.Model, value.IntegrationRevisionID, value.IntegrationManifestHash, value.PromptVersion, value.PromptHash, value.CreatedBy))
}

func (p *Postgres) CreateRecipeWithRevision(ctx context.Context, recipe model.Recipe, revision model.RecipeRevision, mutation RecipeMutation) (model.Recipe, error) {
	var err error
	recipe, err = prepareRecipeRecord(recipe)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := validateRecipeMutation(mutation); err != nil {
		return model.Recipe{}, err
	}
	if mutation.ExpectedRevision != 0 {
		return model.Recipe{}, ErrConflict
	}
	if mutation.Audit == nil {
		return model.Recipe{}, errors.New("recipe creation requires an audit event")
	}
	if revision.RecipeID != recipe.ID {
		return model.Recipe{}, ErrConflict
	}
	revision, err = prepareRecipeRevisionRecord(recipe, revision)
	if err != nil {
		return model.Recipe{}, err
	}
	if revision.Revision != 0 && revision.Revision != 1 {
		return model.Recipe{}, ErrConflict
	}
	prior, current, auditOutcome, err := prepareRecipeAudit(recipe, mutation.Audit, "recipe.created")
	if err != nil {
		return model.Recipe{}, err
	}
	revision.Revision = 1
	recipe.CurrentRevisionID, recipe.CurrentRevision = "", nil
	dependencies, _ := json.Marshal(recipe.Dependencies)

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Recipe{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRecipeCatalog(ctx, tx, recipe.ProductID, mutation.ExpectedCatalogRevision); err != nil {
		return model.Recipe{}, err
	}
	if revision.IntegrationRevisionID != "" {
		var integrationID, manifestHash string
		if err := tx.QueryRow(ctx, `SELECT integration_id::text,manifest_hash FROM integration_revisions WHERE id=$1`, revision.IntegrationRevisionID).Scan(&integrationID, &manifestHash); err != nil {
			return model.Recipe{}, databaseError(err)
		}
		if integrationID != recipe.IntegrationID || manifestHash != revision.IntegrationManifestHash {
			return model.Recipe{}, ErrConflict
		}
	}
	saved, err := createRecipeRow(ctx, tx, recipe, dependencies)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := replaceRecipeAPIAttachments(ctx, tx, recipe, revision.CreatedBy); err != nil {
		return model.Recipe{}, err
	}
	created, err := createRecipeRevisionRow(ctx, tx, revision)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := insertRecipeAPIBindings(ctx, tx, recipe, revision); err != nil {
		return model.Recipe{}, err
	}
	created.APIBindings = append([]model.RecipeAPIBinding(nil), revision.APIBindings...)
	result, err := tx.Exec(ctx, `UPDATE recipes SET current_revision_id=$3 WHERE product_id=$1 AND id=$2 AND current_revision_id IS NULL`, saved.ProductID, saved.ID, created.ID)
	if err != nil {
		return model.Recipe{}, databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return model.Recipe{}, ErrConflict
	}
	if mutation.Audit != nil {
		if err := insertRecipeAudit(ctx, tx, *mutation.Audit, prior, current, auditOutcome); err != nil {
			return model.Recipe{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Recipe{}, databaseError(err)
	}
	saved.CurrentRevisionID = created.ID
	saved.CurrentRevision = &created
	saved.APIAttachments = append([]model.RecipeAPIAttachment(nil), recipe.APIAttachments...)
	return saved, nil
}

func (p *Postgres) SaveRecipeRevision(ctx context.Context, recipe model.Recipe, value model.RecipeRevision, mutation RecipeMutation) (model.Recipe, error) {
	if value.RecipeID != recipe.ID {
		return model.Recipe{}, ErrConflict
	}
	if err := validateRecipeMutation(mutation); err != nil {
		return model.Recipe{}, err
	}
	if mutation.Audit == nil {
		return model.Recipe{}, errors.New("recipe revision changes require an audit event")
	}
	value, err := prepareRecipeRevisionRecord(recipe, value)
	if err != nil {
		return model.Recipe{}, err
	}
	prior, current, auditOutcome, err := prepareRecipeAudit(recipe, mutation.Audit, "recipe.regrounded", "recipe.reworked", "recipe.references.updated")
	if err != nil {
		return model.Recipe{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Recipe{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRecipeCatalog(ctx, tx, recipe.ProductID, mutation.ExpectedCatalogRevision); err != nil {
		return model.Recipe{}, err
	}
	stored, err := scanRecipe(tx.QueryRow(ctx, recipeSelect+` WHERE product_id=$1 AND id=$2 FOR UPDATE`, recipe.ProductID, recipe.ID))
	if err != nil {
		return model.Recipe{}, err
	}
	if stored.Revision != mutation.ExpectedRevision {
		return model.Recipe{}, ErrConflict
	}
	if err := validateRecipeRevisionChange(stored, recipe); err != nil {
		return model.Recipe{}, err
	}
	if value.IntegrationRevisionID != "" {
		var integrationID, manifestHash string
		if err := tx.QueryRow(ctx, `SELECT integration_id::text,manifest_hash FROM integration_revisions WHERE id=$1`, value.IntegrationRevisionID).Scan(&integrationID, &manifestHash); err != nil {
			return model.Recipe{}, databaseError(err)
		}
		if integrationID != recipe.IntegrationID || manifestHash != value.IntegrationManifestHash {
			return model.Recipe{}, ErrConflict
		}
	}
	created, err := createRecipeRevisionRow(ctx, tx, value)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := replaceRecipeAPIAttachments(ctx, tx, recipe, value.CreatedBy); err != nil {
		return model.Recipe{}, err
	}
	if err := insertRecipeAPIBindings(ctx, tx, recipe, value); err != nil {
		return model.Recipe{}, err
	}
	created.APIBindings = append([]model.RecipeAPIBinding(nil), value.APIBindings...)
	recipe.CurrentRevisionID, recipe.CurrentRevision = created.ID, nil
	dependencies, _ := json.Marshal(recipe.Dependencies)
	saved, err := updateRecipeRow(ctx, tx, recipe, dependencies, mutation.ExpectedRevision)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return model.Recipe{}, ErrConflict
		}
		return model.Recipe{}, err
	}
	if stored.State == "published" {
		if err := bumpDeploymentCatalog(ctx, tx, saved.ProductID); err != nil {
			return model.Recipe{}, err
		}
	}
	if mutation.Audit != nil {
		if err := insertRecipeAudit(ctx, tx, *mutation.Audit, prior, current, auditOutcome); err != nil {
			return model.Recipe{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Recipe{}, databaseError(err)
	}
	saved.CurrentRevision = &created
	saved.APIAttachments = append([]model.RecipeAPIAttachment(nil), recipe.APIAttachments...)
	return saved, nil
}

func lockRecipeCatalog(ctx context.Context, tx pgx.Tx, productID string, expected int64) error {
	var deploymentRevision int64
	if err := tx.QueryRow(ctx, `SELECT catalog_revision FROM deployments WHERE id=$1 FOR UPDATE`, productID).Scan(&deploymentRevision); err != nil {
		return databaseError(err)
	}
	var productRevision int64
	if err := tx.QueryRow(ctx, `SELECT catalog_revision FROM products WHERE id=$1 FOR UPDATE`, productID).Scan(&productRevision); err != nil {
		return databaseError(err)
	}
	if deploymentRevision != expected || productRevision != expected {
		return ErrCatalogConflict
	}
	return nil
}

func insertRecipeAudit(ctx context.Context, tx pgx.Tx, audit model.AuditEvent, prior, current []byte, outcome string) error {
	result, err := tx.Exec(ctx, `INSERT INTO audit_events(event_key, organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at) VALUES ($1, nullif($2, '')::uuid, nullif($3, '')::uuid, $4, 'root', $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT (event_key) DO NOTHING`, audit.ID, audit.OrganisationID, audit.ProductID, audit.ActorID, audit.Action, audit.TargetType, audit.TargetID, prior, current, audit.RequestID, outcome, audit.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
