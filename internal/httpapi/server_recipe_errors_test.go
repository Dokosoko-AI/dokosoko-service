package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestRecipePublicVisibilityErrorIsDeterministicClientError(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	new(Server).recipeUpdateError(recorder, platform.ErrPublicMCPRecipe)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "recipe_public_visibility_unsupported" {
		t.Fatalf("error code = %q, body = %s", response.Error.Code, recorder.Body.String())
	}
}

func TestRecipeCatalogConflictIsDeterministicConflict(t *testing.T) {
	t.Parallel()
	for _, err := range []error{store.ErrCatalogConflict, errors.Join(platform.ErrRecipeGroundingChanged, store.ErrCatalogConflict)} {
		recorder := httptest.NewRecorder()
		new(Server).recipeUpdateError(recorder, err)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("error %v: status = %d, body = %s", err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRecipeDeletionRestrictionIsDeterministicClientError(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	new(Server).recipeUpdateError(recorder, platform.ErrRecipeDeletionNotAllowed)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"recipe_delete_not_allowed"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
