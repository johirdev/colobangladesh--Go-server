package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// AdminListUsers godoc
//
//	@Summary	List users
//	@Tags		8. Admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.AdminUserQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.AdminUserPage}
//	@Failure	403		{object}	response.ErrorResponse	"FORBIDDEN"
//	@Router		/admin/users [get]
func (h *Handler) AdminListUsers(c *gin.Context) {
	var q dto.AdminUserQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminListUsers(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// AdminGetUser godoc
//
//	@Summary	Get a user
//	@Tags		8. Admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"User ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=dto.MeResponse}
//	@Failure	404	{object}	response.ErrorResponse	"USER_NOT_FOUND"
//	@Router		/admin/users/{id} [get]
func (h *Handler) AdminGetUser(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.AdminGetUser(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// AdminSetUserStatus godoc
//
//	@Summary		Suspend / re-activate a user
//	@Description	Suspending signs the user out of every device immediately.
//	@Tags			8. Admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"User ID"	format(uuid)
//	@Param			body	body		dto.UpdateUserStatusRequest	true	"New status"
//	@Success		200		{object}	response.Body{data=dto.MeResponse}
//	@Failure		403		{object}	response.ErrorResponse	"CANNOT_MODIFY_ADMIN"
//	@Router			/admin/users/{id}/status [patch]
func (h *Handler) AdminSetUserStatus(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req dto.UpdateUserStatusRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.AdminSetUserStatus(c.Request.Context(), uid(c), id, req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "status updated", res)
}

// AdminListVerifications godoc
//
//	@Summary		Guide KYC review queue
//	@Description	Oldest first. Default: all statuses; use `status=pending` for the queue.
//	@Tags			8. Admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			query	query		dto.VerificationQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.AdminVerificationPage}
//	@Router			/admin/guide-verifications [get]
func (h *Handler) AdminListVerifications(c *gin.Context) {
	var q dto.VerificationQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminListVerifications(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// AdminGetVerification godoc
//
//	@Summary		KYC detail (decrypted NID + signed document URLs)
//	@Description	Document URLs expire after 10 minutes. Every view is written to the audit log.
//	@Tags			8. Admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Verification ID"	format(uuid)
//	@Success		200	{object}	response.Body{data=dto.AdminVerificationDetailResponse}
//	@Failure		404	{object}	response.ErrorResponse	"VERIFICATION_NOT_FOUND"
//	@Router			/admin/guide-verifications/{id} [get]
func (h *Handler) AdminGetVerification(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.AdminGetVerification(c.Request.Context(), uid(c), id, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.OK(c, http.StatusOK, "", res)
}

// AdminApproveVerification godoc
//
//	@Summary	Approve a guide (profile goes live)
//	@Tags		8. Admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Verification ID"	format(uuid)
//	@Success	200	{object}	response.Body
//	@Failure	409	{object}	response.ErrorResponse	"VERIFICATION_ALREADY_REVIEWED"
//	@Router		/admin/guide-verifications/{id}/approve [post]
func (h *Handler) AdminApproveVerification(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.AdminApproveVerification(c.Request.Context(), uid(c), id, meta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "guide approved", nil)
}

// AdminRejectVerification godoc
//
//	@Summary		Reject a guide verification
//	@Description	The reason is shown to the guide, who can fix it and resubmit.
//	@Tags			8. Admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string							true	"Verification ID"	format(uuid)
//	@Param			body	body		dto.RejectVerificationRequest	true	"Reason"
//	@Success		200		{object}	response.Body
//	@Failure		409		{object}	response.ErrorResponse	"VERIFICATION_ALREADY_REVIEWED"
//	@Router			/admin/guide-verifications/{id}/reject [post]
func (h *Handler) AdminRejectVerification(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req dto.RejectVerificationRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.AdminRejectVerification(c.Request.Context(), uid(c), id, req.Reason, meta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "verification rejected", nil)
}

// AdminSetUserRole godoc
//
//	@Summary		Make a user a moderator (or back to tourist)
//	@Description	Moderators can manage divisions, districts, upazilas and tourist spots (/manage/*).
//	@Description	Only `user` ⇄ `moderator` is allowed; guide and admin accounts cannot change role.
//	@Description	The user is signed out everywhere and must log in again to get the new role.
//	@Tags			8. Admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"User ID"	format(uuid)
//	@Param			body	body		dto.UpdateUserRoleRequest	true	"New role"
//	@Success		200		{object}	response.Body{data=dto.MeResponse}
//	@Failure		404		{object}	response.ErrorResponse	"USER_NOT_FOUND"
//	@Failure		409		{object}	response.ErrorResponse	"ROLE_NOT_CHANGEABLE"
//	@Router			/admin/users/{id}/role [patch]
func (h *Handler) AdminSetUserRole(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req dto.UpdateUserRoleRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.AdminSetUserRole(c.Request.Context(), uid(c), id, req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "role updated, the user must log in again", res)
}
