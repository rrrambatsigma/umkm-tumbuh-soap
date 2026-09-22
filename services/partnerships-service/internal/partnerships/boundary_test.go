package partnerships

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/apperror"
)

type boundaryRepository struct {
	Repository
	calls              int
	operation, account string
	status             *PartnershipStatus
	limit, offset      int
	ctx                context.Context
	row                PartnershipResponse
	err                error
}

func (r *boundaryRepository) record(ctx context.Context, operation, account string) {
	r.calls++
	r.ctx, r.operation, r.account = ctx, operation, account
}
func (r *boundaryRepository) FindByRequesterID(ctx context.Context, id string, status *PartnershipStatus, limit, offset int) ([]PartnershipListResponse, int, error) {
	r.record(ctx, "sent", id)
	r.status, r.limit, r.offset = status, limit, offset
	return []PartnershipListResponse{{ID: "P1"}}, 7, r.err
}
func (r *boundaryRepository) FindByReceiverID(ctx context.Context, id string, status *PartnershipStatus, limit, offset int) ([]PartnershipListResponse, int, error) {
	r.record(ctx, "received", id)
	r.status, r.limit, r.offset = status, limit, offset
	return []PartnershipListResponse{{ID: "P1"}}, 7, r.err
}
func (r *boundaryRepository) GetSummary(ctx context.Context, id string) (map[string]int, error) {
	r.record(ctx, "outgoing summary", id)
	return map[string]int{"AKTIF": 3}, r.err
}
func (r *boundaryRepository) GetIncomingSummary(ctx context.Context, id string) (map[string]int, error) {
	r.record(ctx, "incoming summary", id)
	return map[string]int{"AKTIF": 3}, r.err
}
func (r *boundaryRepository) FindByID(ctx context.Context, id string) (*PartnershipResponse, error) {
	r.record(ctx, "detail", id)
	return &r.row, r.err
}

func TestScopedQueriesRequireActor(t *testing.T) {
	status := StatusSubmitted
	operations := []struct {
		name string
		call func(Service, context.Context, string) error
	}{
		{"sent", func(s Service, ctx context.Context, id string) error {
			_, _, err := s.GetPartnershipsByRequester(ctx, id, &status, 3, 10)
			return err
		}},
		{"received", func(s Service, ctx context.Context, id string) error {
			_, _, err := s.GetPartnershipsByReceiver(ctx, id, &status, 3, 10)
			return err
		}},
		{"outgoing summary", func(s Service, ctx context.Context, id string) error {
			_, err := s.GetPartnershipSummary(ctx, id)
			return err
		}},
		{"incoming summary", func(s Service, ctx context.Context, id string) error {
			_, err := s.GetIncomingPartnershipSummary(ctx, id)
			return err
		}},
	}
	for _, op := range operations {
		for _, tc := range []struct {
			name  string
			ctx   context.Context
			scope string
			want  int
		}{
			{"missing actor", context.Background(), "account", 401},
			{"spoofed key", context.WithValue(context.Background(), "user_id", "account"), "account", 401},
			{"blank actor", actorContext(" ", "UMKM"), "account", 401},
			{"invalid role", actorContext("account", "OWNER"), "account", 403},
			{"admin", actorContext("account", "ADMIN"), "account", 403},
			{"foreign scope", actorContext("account", "UMKM"), "victim", 403},
			{"empty scope", actorContext("account", "UMKM"), "", 403},
			{"umkm", actorContext("account", "UMKM"), "account", 0},
			{"mitra", actorContext("account", "MITRA"), "account", 0},
		} {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				repo := &boundaryRepository{}
				err := op.call(NewService(repo), tc.ctx, tc.scope)
				requireStatus(t, err, tc.want)
				if tc.want != 0 {
					if repo.calls != 0 {
						t.Fatal("unauthorized query reached repository")
					}
					return
				}
				if repo.calls != 1 || repo.operation != op.name || repo.account != "account" || repo.ctx != tc.ctx {
					t.Fatalf("incorrect query: %+v", repo)
				}
				if (op.name == "sent" || op.name == "received") && (repo.status != &status || repo.limit != 10 || repo.offset != 20) {
					t.Fatal("filter/pagination changed")
				}
			})
		}
	}
}

func TestReadBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, id, role string
		missing        bool
		want           int
		calls          int
	}{
		{"receiver", "receiver", "MITRA", false, 0, 1},
		{"requester", "requester", "UMKM", false, 403, 1},
		{"outsider", "outsider", "UMKM", false, 403, 1},
		{"invalid role", "receiver", "OWNER", false, 403, 0},
		{"missing actor", "", "", false, 401, 0},
		{"nonexistent", "receiver", "MITRA", true, 404, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &boundaryRepository{row: PartnershipResponse{PartnershipRequest: PartnershipRequest{ID: "P1", RequesterID: "requester", ReceiverID: "receiver", Status: StatusSubmitted}}}
			if tc.missing {
				repo.err = apperror.New(404, "Pengajuan kemitraan tidak ditemukan")
			}
			reader, ok := NewService(repo).(interface {
				MarkPartnershipAsRead(context.Context, string) error
			})
			if !ok {
				t.Fatal("service has no shared read authorization")
			}
			err := reader.MarkPartnershipAsRead(actorContext(tc.id, tc.role), "P1")
			requireStatus(t, err, tc.want)
			if repo.calls != tc.calls || repo.row.Status != StatusSubmitted {
				t.Fatal("read changed state or accessed repository unexpectedly")
			}
			// Mutation methods are deliberately unimplemented: any write would panic.
		})
	}
}

type creationRepository struct {
	profileErr, reloadErr error
	Repository
	calls, writes                       int
	lookupErr, createErr, attachmentErr error
	receiverID                          string
	receiverRole                        UserRole
	created                             PartnershipRequest
}

func (r *creationRepository) GenerateRequestCode(context.Context) (string, error) {
	r.calls++
	return "PKS-test", nil
}
func (r *creationRepository) FindAkunIDByBusinessID(_ context.Context, id string, role UserRole) (string, error) {
	r.calls++
	r.receiverID, r.receiverRole = id, role
	return "receiver", r.lookupErr
}
func (r *creationRepository) FindBusinessIDByAkunID(context.Context, string, UserRole) (string, error) {
	r.calls++
	if r.profileErr != nil {
		return "", r.profileErr
	}
	return "requester-business", nil
}
func (r *creationRepository) CountAll(context.Context) (int, error) { r.calls++; return 0, nil }
func (r *creationRepository) OwnsDocument(context.Context, string, string, bool) (bool, error) {
	r.calls++
	return true, nil
}
func (r *creationRepository) Create(_ context.Context, p *PartnershipRequest) error {
	r.calls++
	r.writes++
	r.created = *p
	return r.createErr
}
func (r *creationRepository) CreateAttachments(context.Context, string, []string) error {
	r.calls++
	return r.attachmentErr
}
func (r *creationRepository) FindByID(context.Context, string) (*PartnershipResponse, error) {
	r.calls++
	return &PartnershipResponse{PartnershipRequest: r.created}, r.reloadErr
}
func validCreation() CreatePartnershipRequest {
	return CreatePartnershipRequest{ReceiverID: "receiver-business", ProposalTitle: "Valid proposal title", ProposalDescription: "A sufficiently long proposal description."}
}

func TestCreateDirectValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*CreatePartnershipRequest)
	}{
		{"receiver", func(r *CreatePartnershipRequest) { r.ReceiverID = "" }},
		{"short title", func(r *CreatePartnershipRequest) { r.ProposalTitle = "short" }},
		{"long title", func(r *CreatePartnershipRequest) { r.ProposalTitle = strings.Repeat("a", 201) }},
		{"short description", func(r *CreatePartnershipRequest) { r.ProposalDescription = "short" }},
		{"long description", func(r *CreatePartnershipRequest) { r.ProposalDescription = strings.Repeat("a", 1001) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &creationRepository{}
			req := validCreation()
			tc.alter(&req)
			_, err := NewService(repo).CreatePartnership(actorContext("requester", "UMKM"), "requester", RoleUMKM, req)
			requireStatus(t, err, 422)
			if repo.calls != 0 {
				t.Fatal("invalid request reached repository")
			}
		})
	}
	for _, role := range []UserRole{RoleUMKM, RoleMitra} {
		t.Run(string(role), func(t *testing.T) {
			repo := &creationRepository{}
			req := validCreation()
			result, err := NewService(repo).CreatePartnership(actorContext("requester", string(role)), "requester", role, req)
			requireStatus(t, err, 0)
			wantReceiverRole := RoleMitra
			if role == RoleMitra {
				wantReceiverRole = RoleUMKM
			}
			if result.ID == "" || repo.writes != 1 || repo.receiverID != req.ReceiverID || repo.receiverRole != wantReceiverRole || repo.created.RequesterID != "requester" || repo.created.ProposalTitle != req.ProposalTitle || repo.created.ProposalDescription != req.ProposalDescription {
				t.Fatalf("creation mapping changed: %+v", repo)
			}
		})
	}
}

func TestCreateInfrastructureErrorsAreSafe(t *testing.T) {
	const sensitive = "SQLSTATE 42P01 password=secret private_table"
	for _, stage := range []string{"lookup", "profile", "create", "attachments", "reload"} {
		t.Run(stage, func(t *testing.T) {
			repo := &creationRepository{}
			switch stage {
			case "profile":
				repo.profileErr = errors.New(sensitive)
			case "reload":
				repo.reloadErr = errors.New(sensitive)
			case "lookup":
				repo.lookupErr = errors.New(sensitive)
			case "create":
				repo.createErr = errors.New(sensitive)
			case "attachments":
				repo.attachmentErr = errors.New(sensitive)
			}
			req := validCreation()
			req.AttachmentFiles = []string{"owned"}
			_, err := NewService(repo).CreatePartnership(actorContext("requester", "UMKM"), "requester", RoleUMKM, req)
			requireStatus(t, err, 500)
			if (stage == "lookup" || stage == "profile") && repo.writes != 0 {
				t.Fatal("lookup failure continued to write")
			}
			if strings.Contains(err.Error(), sensitive) {
				t.Fatalf("public error leaked infrastructure details: %v", err)
			}
		})
	}
}

func TestCreateAllowsMissingOptionalRequesterProfile(t *testing.T) {
	repo := &creationRepository{profileErr: ErrBusinessNotFound}
	_, err := NewService(repo).CreatePartnership(actorContext("requester", "UMKM"), "requester", RoleUMKM, validCreation())
	requireStatus(t, err, 0)
	if repo.writes != 1 || repo.created.RequesterBusinessID != "" {
		t.Fatal("optional profile behavior changed")
	}
}

func TestCreateBoundaryRejectsInvalidActorsAndReceiver(t *testing.T) {
	for _, tc := range []struct {
		name, id, role string
		lookupErr      error
		want           int
	}{
		{"missing actor", "", "UMKM", nil, 401},
		{"invalid role", "requester", "OWNER", nil, 403},
		{"admin", "requester", "ADMIN", nil, 403},
		{"mismatched requester", "victim", "UMKM", nil, 403},
		{"mismatched role", "requester", "MITRA", nil, 403},
		{"missing receiver", "requester", "UMKM", ErrBusinessNotFound, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &creationRepository{lookupErr: tc.lookupErr}
			_, err := NewService(repo).CreatePartnership(actorContext(tc.id, tc.role), "requester", RoleUMKM, validCreation())
			requireStatus(t, err, tc.want)
			if repo.writes != 0 || (tc.lookupErr == nil && repo.calls != 0) {
				t.Fatal("denied creation reached repository or wrote data")
			}
		})
	}
}
