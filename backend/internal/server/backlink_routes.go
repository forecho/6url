package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/backlink"
)

func recheckMySubmission(c *gin.Context, deps Dependencies) {
	session, ok := authenticateApplicationSession(c, deps, "submission")
	if !ok || !validateSessionWrite(c, deps, session) || !allowRequest(c, deps, "submission:recheck", 3, time.Hour) {
		return
	}
	application, err := deps.Ent.Submission.Get(c.Request.Context(), *session.SubmissionID)
	if err != nil || application.Type != submission.TypeExchange || application.BacklinkURL == nil {
		writeError(c, http.StatusConflict, "invalid_state", "The submission has no exchange backlink to check.")
		return
	}
	placementRow, err := deps.Ent.Placement.Query().Where(placement.SubmissionIDEQ(application.ID), placement.KindEQ(placement.KindExchange)).Only(c.Request.Context())
	if ent.IsNotFound(err) {
		result := deps.Backlinks.Check(c.Request.Context(), *application.BacklinkURL, deps.Config.AppBaseURL)
		c.Header("Cache-Control", "private, no-store")
		c.JSON(http.StatusOK, backlinkResponse(result, nil))
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the exchange placement.")
		return
	}
	result, updated, err := executePlacementBacklinkCheck(c, deps, placementRow.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to save the backlink check.")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, backlinkResponse(result, updated))
}

func recheckAdminPlacement(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, updated, err := executePlacementBacklinkCheck(c, deps, id)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, backlink.ErrNotExchangePlacement) {
			writeError(c, http.StatusNotFound, "not_found", "The exchange placement does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to check the exchange backlink.")
		}
		return
	}
	c.JSON(http.StatusOK, backlinkResponse(result, updated))
}

func executePlacementBacklinkCheck(c *gin.Context, deps Dependencies, placementID int) (backlink.Result, *ent.Placement, error) {
	monitor := backlink.Monitor{Client: deps.Ent, Checker: deps.Backlinks, TargetURL: deps.Config.AppBaseURL, Sealer: deps.PayloadSealer, SlackChannelID: deps.Config.SlackChannelID}
	return monitor.CheckPlacement(c.Request.Context(), placementID)
}

func backlinkResponse(result backlink.Result, row *ent.Placement) gin.H {
	response := gin.H{"status": result.Status, "error_code": result.ErrorCode, "checked_at": result.CheckedAt, "final_url": result.FinalURL, "http_status": result.HTTPStatus, "matched_href": result.MatchedHref, "matched_rel": result.MatchedRel}
	if row != nil {
		response["placement_id"] = row.ID
		response["placement_status"] = row.Status
		response["consecutive_failures"] = row.ConsecutiveFailures
	}
	return response
}
