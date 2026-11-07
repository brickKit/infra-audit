package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brickKit/be-sdk-go/v3/db"
)

func EpochOf(ctx context.Context, tx db.Tx) (string, error) {
	var g, t int64
	if err := tx.QueryRow(ctx, "SELECT generation, timeline FROM be_current_db_epoch()").Scan(&g, &t); err != nil {
		return "", err
	}
	return strconv.FormatInt(g, 10) + "." + strconv.FormatInt(t, 10), nil
}

// ChainNext is the source's chain digest after one more record (contract/README.md).
func ChainNext(prev, digest string) string {
	sum := sha256.Sum256([]byte(prev + "\n" + digest))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// recordSkew is how much earlier than its occurrence a record may have been received (the source's clock ahead of
// the store's): the index on the reception time narrows the scan, the occurrence decides.
const RecordSkew = time.Hour

// Position is a record's place in the store: "<epoch>/<storage seq>".
func Position(epoch string, seq int64) string { return epoch + "/" + strconv.FormatInt(seq, 10) }

func ParsePosition(p string) (string, int64, error) {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "", 0, fmt.Errorf("not a position: %q", p)
	}
	seq, err := strconv.ParseInt(p[i+1:], 10, 64)
	return p[:i], seq, err
}
