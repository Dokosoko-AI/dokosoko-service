package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

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

const recipeSelect = `SELECT id::text,organisation_id::text,product_id::text,coalesce(analysis_id::text,''),slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,coalesce(current_revision_id::text,''),stable_uri,approved_by,approved_at,published_at,revision,created_at,updated_at FROM recipes`

func scanRecipe(row pgx.Row) (model.Recipe, error) {
	var value model.Recipe
	var dependencies []byte
	if err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.AnalysisID, &value.Slug, &value.Title, &value.Outcome, &value.Audience, &value.State, &value.Generated, &value.NeedsAttention, &value.Visibility, &dependencies, &value.CurrentRevisionID, &value.StableURI, &value.ApprovedBy, &value.ApprovedAt, &value.PublishedAt, &value.Revision, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return value, databaseError(err)
	}
	if err := json.Unmarshal(dependencies, &value.Dependencies); err != nil {
		return value, err
	}
	return value, nil
}

func (p *Postgres) hydrateRecipe(ctx context.Context, value model.Recipe) (model.Recipe, error) {
	if value.CurrentRevisionID == "" {
		return value, nil
	}
	revision, err := scanRecipeRevision(p.pool.QueryRow(ctx, recipeRevisionSelect+` WHERE recipe_id=$1 AND id=$2`, value.ID, value.CurrentRevisionID))
	if err != nil {
		return value, err
	}
	value.CurrentRevision = &revision
	return value, nil
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

func updateRecipeRow(ctx context.Context, query pgxRowQuerier, value model.Recipe, dependencies []byte, expected int64) (model.Recipe, error) {
	return scanRecipe(query.QueryRow(ctx, `UPDATE recipes SET analysis_id=nullif($3,'')::uuid,title=$4,outcome=$5,audience=$6,state=$7,needs_attention=$8,visibility=$9,dependencies=$10,current_revision_id=nullif($11,'')::uuid,approved_by=$12,approved_at=$13,published_at=$14,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$15 RETURNING id::text,organisation_id::text,product_id::text,coalesce(analysis_id::text,''),slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,coalesce(current_revision_id::text,''),stable_uri,approved_by,approved_at,published_at,revision,created_at,updated_at`, value.ProductID, value.ID, value.AnalysisID, value.Title, value.Outcome, value.Audience, value.State, value.NeedsAttention, value.Visibility, dependencies, value.CurrentRevisionID, value.ApprovedBy, value.ApprovedAt, value.PublishedAt, expected))
}

func (p *Postgres) SaveRecipe(ctx context.Context, value model.Recipe, expected int64) (model.Recipe, error) {
	dependencies, _ := json.Marshal(value.Dependencies)
	var saved model.Recipe
	var err error
	if expected == 0 {
		saved, err = scanRecipe(p.pool.QueryRow(ctx, `INSERT INTO recipes(id,organisation_id,product_id,analysis_id,slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,current_revision_id,stable_uri,approved_by,approved_at,published_at) VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,nullif($14,'')::uuid,$15,$16,$17,$18) RETURNING id::text,organisation_id::text,product_id::text,coalesce(analysis_id::text,''),slug,title,outcome,audience,state,generated,needs_attention,visibility,dependencies,coalesce(current_revision_id::text,''),stable_uri,approved_by,approved_at,published_at,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.AnalysisID, value.Slug, value.Title, value.Outcome, value.Audience, value.State, value.Generated, value.NeedsAttention, value.Visibility, dependencies, value.CurrentRevisionID, value.StableURI, value.ApprovedBy, value.ApprovedAt, value.PublishedAt))
	} else {
		saved, err = updateRecipeRow(ctx, p.pool, value, dependencies, expected)
		if errors.Is(err, ErrNotFound) {
			if _, lookupErr := p.Recipe(ctx, value.ProductID, value.ID); lookupErr == nil {
				return model.Recipe{}, ErrConflict
			}
		}
	}
	if err != nil {
		return model.Recipe{}, err
	}
	return p.hydrateRecipe(ctx, saved)
}

const recipeRevisionSelect = `SELECT id::text,recipe_id::text,revision,markdown,reference_items,validation,review,generated_by,model,created_by,created_at FROM recipe_revisions`

func scanRecipeRevision(row pgx.Row) (model.RecipeRevision, error) {
	var value model.RecipeRevision
	var references, validation []byte
	if err := row.Scan(&value.ID, &value.RecipeID, &value.Revision, &value.Markdown, &references, &validation, &value.Review, &value.GeneratedBy, &value.Model, &value.CreatedBy, &value.CreatedAt); err != nil {
		return value, databaseError(err)
	}
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
	return values, rows.Err()
}

func createRecipeRevisionRow(ctx context.Context, query pgxRowQuerier, value model.RecipeRevision) (model.RecipeRevision, error) {
	references, _ := json.Marshal(value.References)
	validation, _ := json.Marshal(value.Validation)
	return scanRecipeRevision(query.QueryRow(ctx, `INSERT INTO recipe_revisions(id,recipe_id,revision,markdown,reference_items,validation,review,generated_by,model,created_by) VALUES($1,$2,coalesce(nullif($3,0),(SELECT coalesce(max(revision),0)+1 FROM recipe_revisions WHERE recipe_id=$2)),$4,$5,$6,$7,$8,$9,$10) RETURNING id::text,recipe_id::text,revision,markdown,reference_items,validation,review,generated_by,model,created_by,created_at`, value.ID, value.RecipeID, value.Revision, value.Markdown, references, validation, value.Review, value.GeneratedBy, value.Model, value.CreatedBy))
}

func (p *Postgres) SaveRecipeRevision(ctx context.Context, recipe model.Recipe, value model.RecipeRevision, expected int64) (model.Recipe, error) {
	if value.RecipeID != recipe.ID {
		return model.Recipe{}, ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Recipe{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := createRecipeRevisionRow(ctx, tx, value)
	if err != nil {
		return model.Recipe{}, err
	}
	recipe.CurrentRevisionID, recipe.CurrentRevision = created.ID, nil
	dependencies, _ := json.Marshal(recipe.Dependencies)
	saved, err := updateRecipeRow(ctx, tx, recipe, dependencies, expected)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			_ = tx.Rollback(ctx)
			if _, lookupErr := p.Recipe(ctx, recipe.ProductID, recipe.ID); lookupErr == nil {
				return model.Recipe{}, ErrConflict
			}
		}
		return model.Recipe{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Recipe{}, databaseError(err)
	}
	saved.CurrentRevision = &created
	return saved, nil
}

const aiJobSelect = `SELECT id::text,organisation_id::text,product_id::text,coalesce(kind,''),coalesce(target_id::text,''),state,attempt,input,output,error_code,created_by,created_at,started_at,finished_at FROM ai_jobs`

func scanAIJob(row pgx.Row) (model.AIJob, error) {
	var value model.AIJob
	if err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Kind, &value.TargetID, &value.State, &value.Attempt, &value.Input, &value.Output, &value.ErrorCode, &value.CreatedBy, &value.CreatedAt, &value.StartedAt, &value.FinishedAt); err != nil {
		return value, databaseError(err)
	}
	return value, nil
}

func (p *Postgres) AIJobs(ctx context.Context, productID string) ([]model.AIJob, error) {
	rows, err := p.pool.Query(ctx, aiJobSelect+` WHERE product_id=$1 ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.AIJob, 0)
	for rows.Next() {
		value, err := scanAIJob(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) AIJob(ctx context.Context, productID, id string) (model.AIJob, error) {
	return scanAIJob(p.pool.QueryRow(ctx, aiJobSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) SaveAIJob(ctx context.Context, value model.AIJob) (model.AIJob, error) {
	if len(value.Input) == 0 {
		value.Input = json.RawMessage(`{}`)
	}
	if len(value.Output) == 0 {
		value.Output = json.RawMessage(`{}`)
	}
	return scanAIJob(p.pool.QueryRow(ctx, `INSERT INTO ai_jobs(id,organisation_id,product_id,kind,target_id,state,attempt,input,output,error_code,created_by,started_at,finished_at) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(id) DO UPDATE SET target_id=excluded.target_id,state=excluded.state,attempt=excluded.attempt,input=excluded.input,output=excluded.output,error_code=excluded.error_code,started_at=excluded.started_at,finished_at=excluded.finished_at RETURNING id::text,organisation_id::text,product_id::text,kind,coalesce(target_id::text,''),state,attempt,input,output,error_code,created_by,created_at,started_at,finished_at`, value.ID, value.OrganisationID, value.ProductID, value.Kind, value.TargetID, value.State, value.Attempt, value.Input, value.Output, value.ErrorCode, value.CreatedBy, value.StartedAt, value.FinishedAt))
}
