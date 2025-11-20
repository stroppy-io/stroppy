package quota

import (
	"errors"
	"fmt"
	"strings"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

type QName string

func newQName(cloud crossplane.SupportedCloud, kind crossplane.Quota_Kind) QName {
	return QName(fmt.Sprintf("%s:%s", cloud.String(), kind.String()))
}
func fromName(name QName) (crossplane.SupportedCloud, crossplane.Quota_Kind) {
	parts := strings.Split(string(name), ":")
	return crossplane.SupportedCloud(crossplane.SupportedCloud_value[parts[0]]),
		crossplane.Quota_Kind(crossplane.Quota_Kind_value[parts[1]])
}

type ExceededError struct {
	Quota *crossplane.Quota
}

func NewExceededError(quota *crossplane.Quota) *ExceededError {
	return &ExceededError{Quota: quota}
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf(
		"quota %s:%s exceeded max limit: %d/%d",
		e.Quota.GetCloud().String(),
		e.Quota.GetKind(),
		e.Quota.GetMaximum(),
		e.Quota.GetCurrent(),
	)
}

func IsExceededError(err error) bool {
	var e *ExceededError
	return errors.As(err, &e)
}

func QuotasToKinds(quotas []*crossplane.Quota) []crossplane.Quota_Kind {
	var names []crossplane.Quota_Kind
	for _, q := range quotas {
		names = append(names, q.GetKind())
	}
	return names
}

func CheckQuotasExceeded(needed []*crossplane.Quota, available []*crossplane.Quota) error {
	mapNeeded := make(map[QName]uint32)
	for _, q := range needed {
		mapNeeded[newQName(q.GetCloud(), q.GetKind())] = q.GetCurrent() // current means needed
	}
	for _, q := range available {
		if q.GetCurrent()+mapNeeded[newQName(q.GetCloud(), q.GetKind())] > q.GetMaximum() {
			return NewExceededError(q)
		}
	}
	return nil
}
