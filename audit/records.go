package audit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
)

// recordSkew is how much earlier than its occurrence a record may have been received (the source's clock ahead of
// the store's): the index on the reception time narrows the scan, the occurrence decides.
const recordSkew = time.Hour

// recordList answers RecordService (contract alpha.3) for the configured readers.
type recordList struct {
	auditv3.UnimplementedRecordServiceServer
	m *Module
}

// position is a record's place in the store: "<epoch>/<storage seq>".
func position(epoch string, seq int64) string { return epoch + "/" + strconv.FormatInt(seq, 10) }

func parsePosition(p string) (string, int64, error) {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "", 0, fmt.Errorf("not a position: %q", p)
	}
	seq, err := strconv.ParseInt(p[i+1:], 10, 64)
	return p[:i], seq, err
}

// List is RecordService.List: the records that occurred at or after since, by category, in the store's order after a
// position, 500 a page at most (a full recovery's loss range, assembly protocol 15 step 9).
func (r recordList) List(ctx context.Context, in *auditv3.RecordServiceListRequest) (*auditv3.RecordServiceListResponse, error) {
	since, err := time.Parse(time.RFC3339Nano, in.GetSince())
	if err != nil {
		return nil, errs.New(errs.ReasonRequestInvalid, "since is not an RFC 3339 time")
	}
	epoch, seq := "", int64(-1)
	if in.GetAfter() != "" {
		if epoch, seq, err = parsePosition(in.GetAfter()); err != nil {
			return nil, errs.New(errs.ReasonRequestInvalid, err.Error())
		}
	}
	limit := int(in.GetLimit())
	if limit <= 0 || limit > MaxBatch {
		limit = MaxBatch
	}
	categories := in.GetCategories()
	out := &auditv3.RecordServiceListResponse{}
	err = r.m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		out.Records = out.Records[:0]
		rows, err := tx.Query(ctx, `SELECT source, record, source_epoch, source_seq, epoch, storage_seq FROM audit_record
			WHERE received_at >= $1::timestamptz - make_interval(secs => $2)
			  AND (convert_from(record, 'UTF8')::jsonb ->> 'occurred_at')::timestamptz >= $1::timestamptz
			  AND (coalesce(cardinality($3::text[]), 0) = 0 OR convert_from(record, 'UTF8')::jsonb ->> 'category' = ANY($3))
			  AND (epoch, storage_seq) > ($4, $5)
			ORDER BY epoch, storage_seq LIMIT $6`, since, recordSkew.Seconds(), categories, epoch, seq, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s auditv3.StoredRecord
			var ep string
			var sq int64
			if err := rows.Scan(&s.Source, &s.Record, &s.SourceEpoch, &s.SourceSeq, &ep, &sq); err != nil {
				return err
			}
			s.Position = position(ep, sq)
			out.Records = append(out.Records, &s)
		}
		return rows.Err()
	})
	if len(out.Records) > limit {
		out.Records, out.More = out.Records[:limit], true
	}
	return out, err
}
