package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

func developerAssetCreationTx(ctx context.Context, tx pgx.Tx, request *DeveloperAssetCreation) (string, error) {
	if request == nil {
		return "", nil
	}
	event := request.Audit
	// A short deployment lock serializes create/retry; evidence processing and
	// external requests happen before this transaction.
	var deploymentID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM deployments WHERE id=$1 AND organisation_id=$2 FOR UPDATE`, event.ProductID, event.OrganisationID).Scan(&deploymentID); err != nil {
		return "", databaseError(err)
	}
	var previous developerAssetCreationResult
	err := tx.QueryRow(ctx, `SELECT COALESCE(api_contract_id,documentation_collection_id)::text,input_digest FROM developer_asset_creation_requests WHERE deployment_id=$1 AND resource_kind=$2 AND request_digest=$3`, event.ProductID, event.TargetType, request.RequestDigest).Scan(&previous.ResourceID, &previous.InputDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", databaseError(err)
	}
	if previous.InputDigest != request.InputDigest {
		return "", ErrDeveloperAssetCreationConflict
	}
	return previous.ResourceID, nil
}

func saveDeveloperAssetCreationTx(ctx context.Context, tx pgx.Tx, request *DeveloperAssetCreation) error {
	if request == nil {
		return nil
	}
	event := request.Audit
	var contractID, collectionID any
	if event.TargetType == "api_contract" {
		contractID = event.TargetID
	} else {
		collectionID = event.TargetID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO developer_asset_creation_requests(deployment_id,resource_kind,request_digest,input_digest,api_contract_id,documentation_collection_id) VALUES($1,$2,$3,$4,$5,$6)`, event.ProductID, event.TargetType, request.RequestDigest, request.InputDigest, contractID, collectionID); err != nil {
		return databaseError(err)
	}
	current, err := json.Marshal(event.Current)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(event_key,organisation_id,product_id,actor_id,actor_kind,action,target_type,target_id,prior,current,request_id,outcome,created_at) VALUES($1,$2,$3,$4,'root',$5,$6,$7,'null',$8,$9,'success',$10)`, event.ID, event.OrganisationID, event.ProductID, event.ActorID, event.Action, event.TargetType, event.TargetID, current, event.RequestID, event.CreatedAt)
	return databaseError(err)
}
