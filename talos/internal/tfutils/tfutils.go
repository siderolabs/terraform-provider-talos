// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tfutils

import (
	"context"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type typesD interface {
	types.Bool | types.Float64 | types.Int64 | types.List | types.Map | types.Number | types.Object | types.Set | types.String
}

type defaultValuePlanModifier[T typesD] struct {
	DefaultValue T
}

func DefaultValue[T typesD](v T) any {
	return &defaultValuePlanModifier[T]{DefaultValue: v}
}

var _ planmodifier.Bool = (*defaultValuePlanModifier[types.Bool])(nil)
var _ planmodifier.Float64 = (*defaultValuePlanModifier[types.Float64])(nil)
var _ planmodifier.Int64 = (*defaultValuePlanModifier[types.Int64])(nil)
var _ planmodifier.List = (*defaultValuePlanModifier[types.List])(nil)
var _ planmodifier.Map = (*defaultValuePlanModifier[types.Map])(nil)
var _ planmodifier.Number = (*defaultValuePlanModifier[types.Number])(nil)
var _ planmodifier.Object = (*defaultValuePlanModifier[types.Object])(nil)
var _ planmodifier.Set = (*defaultValuePlanModifier[types.Set])(nil)
var _ planmodifier.String = (*defaultValuePlanModifier[types.String])(nil)

func (apm *defaultValuePlanModifier[typesD]) Description(ctx context.Context) string {
	return ""
}

func (apm *defaultValuePlanModifier[typesD]) MarkdownDescription(ctx context.Context) string {
	return ""
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, res *planmodifier.BoolResponse) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyFloat64(ctx context.Context, req planmodifier.Float64Request, res *planmodifier.Float64Response) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, res *planmodifier.Int64Response) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, res *planmodifier.ListResponse) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, res *planmodifier.StringResponse) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyMap(ctx context.Context, req planmodifier.MapRequest, res *planmodifier.MapResponse) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyNumber(ctx context.Context, req planmodifier.NumberRequest, res *planmodifier.NumberResponse) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, res *planmodifier.ObjectResponse) {
}

func (apm *defaultValuePlanModifier[typesD]) PlanModifySet(ctx context.Context, req planmodifier.SetRequest, res *planmodifier.SetResponse) {
}

// tfTypesToInterface converts a Terraform specific tftypes.Value type object
// into an interface{} type object.
func TFTypesToInterface(in tftypes.Value, ap *tftypes.AttributePath) (interface{}, error) {
	var err error
	if !in.IsKnown() {
		return nil, ap.NewErrorf("[%s] cannot convert unknown value to Unstructured", ap.String())
	}

	if in.IsNull() {
		return nil, nil
	}

	if in.Type().Is(tftypes.DynamicPseudoType) {
		return nil, ap.NewErrorf("[%s] cannot convert dynamic value to Unstructured", ap.String())
	}

	switch {
	case in.Type().Is(tftypes.Bool):
		var bv bool
		err = in.As(&bv)
		if err != nil {
			return nil, ap.NewErrorf("[%s] cannot extract contents of attribute: %s", ap.String(), err)
		}

		return bv, nil
	case in.Type().Is(tftypes.Number):
		var nv big.Float
		err = in.As(&nv)
		if nv.IsInt() {
			inv, acc := nv.Int64()
			if acc != big.Exact {
				return nil, ap.NewErrorf("[%s] inexact integer approximation when converting number value at:", ap.String())
			}

			return inv, nil
		}
		fnv, _ := nv.Float64()

		return fnv, err
	case in.Type().Is(tftypes.String):
		var sv string
		err = in.As(&sv)
		if err != nil {
			return nil, ap.NewErrorf("[%s] cannot extract contents of attribute: %s", ap.String(), err)
		}

		return sv, nil
	case in.Type().Is(tftypes.List{}) || in.Type().Is(tftypes.Tuple{}):
		var l []tftypes.Value
		var lv []interface{}
		err = in.As(&l)
		if err != nil {
			return nil, ap.NewErrorf("[%s] cannot extract contents of attribute: %s", ap.String(), err)
		}
		if len(l) == 0 {
			return lv, nil
		}
		for k, le := range l {
			nextAp := ap.WithElementKeyInt(k)
			ne, err := TFTypesToInterface(le, nextAp)
			if err != nil {
				return nil, nextAp.NewErrorf("[%s] cannot convert list element to Unstructured: %s", nextAp.String(), err)
			}
			if ne != nil {
				lv = append(lv, ne)
			}
		}

		return lv, nil
	case in.Type().Is(tftypes.Map{}) || in.Type().Is(tftypes.Object{}):
		m := make(map[string]tftypes.Value)
		mv := make(map[string]interface{})
		err = in.As(&m)
		if err != nil {
			return nil, ap.NewErrorf("[%s] cannot extract contents of attribute: %s", ap.String(), err)
		}
		if len(m) == 0 {
			return mv, nil
		}
		for k, me := range m {
			var nextAp *tftypes.AttributePath
			switch {
			case in.Type().Is(tftypes.Map{}):
				nextAp = ap.WithElementKeyString(k)
			case in.Type().Is(tftypes.Object{}):
				nextAp = ap.WithAttributeName(k)
			}
			ne, err := TFTypesToInterface(me, nextAp)
			if err != nil {
				return nil, nextAp.NewErrorf("[%s]: cannot convert map element to Unstructured: %s", nextAp.String(), err.Error())
			}
			mv[k] = ne
		}

		return mv, nil
	default:
		return nil, ap.NewErrorf("[%s] cannot convert value of unknown type (%s)", ap.String(), in.Type().String())
	}
}
