// Package properties is the model of CSS property values: the known
// properties ([KnownProp], [Shorthand]), their value types ([CssProperty]
// and its implementations), the style interfaces the cascade fills
// ([StyleAccessor]), and the tables describing the properties, read
// through functions ([InitialValue], [InitialStyle], [IsInherited],
// [PropertyByName], ...).
//
// The tables cannot be modified from outside the package. The values they
// hold (slices included, as in [Strings]) are shared by every style of
// every document, and must never be modified in place: copy a value before
// changing it.
//
// The model is ported from WeasyPrint (https://github.com/Kozea/WeasyPrint);
// part of it is generated (go generate) from properties_src.go.
package properties
