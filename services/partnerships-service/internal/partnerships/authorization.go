package partnerships

import (
	"context"
	"net/http"

	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/apperror"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/auth"
)

func partnershipActor(ctx context.Context) (string, UserRole, error) {
	actor, ok := auth.ActorFromContext(ctx)
	if !ok {
		return "", "", apperror.New(http.StatusUnauthorized, "User belum terautentikasi")
	}
	role := actor.Role
	if role != string(RoleUMKM) && role != string(RoleMitra) {
		return "", "", apperror.New(http.StatusForbidden, "Kemitraan hanya untuk UMKM dan Mitra")
	}
	return actor.UserID, UserRole(role), nil
}

// partnershipScope verifies the compatibility argument but returns only the
// authenticated account ID as the repository query scope.
func partnershipScope(ctx context.Context, requestedID string) (string, error) {
	actorID, _, err := partnershipActor(ctx)
	if err != nil {
		return "", err
	}
	if requestedID != actorID {
		return "", apperror.New(http.StatusForbidden, "Identitas akun tidak sesuai sesi")
	}
	return actorID, nil
}

func authorizeStatusChange(p *PartnershipResponse, actorID string, next PartnershipStatus) error {
	switch next {
	case StatusActive, StatusRejected:
		if p.ReceiverID != actorID {
			return apperror.New(http.StatusForbidden, "Hanya penerima yang dapat memutuskan pengajuan")
		}
		if p.Status != StatusSubmitted && p.Status != StatusReviewed {
			return apperror.New(http.StatusConflict, "Pengajuan tidak berada pada status yang dapat diputuskan")
		}
	case StatusCancelled:
		if p.RequesterID != actorID {
			return apperror.New(http.StatusForbidden, "Hanya pengaju yang dapat membatalkan pengajuan")
		}
		if p.Status != StatusDraft && p.Status != StatusSubmitted && p.Status != StatusReviewed {
			return apperror.New(http.StatusConflict, "Pengajuan tidak dapat dibatalkan pada status ini")
		}
	default:
		return apperror.New(http.StatusBadRequest, "Perubahan status tidak diizinkan")
	}
	return nil
}
