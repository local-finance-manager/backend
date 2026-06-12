package category

import "context"

// SubcategoryFacade satisfies the transaction.SubcategoryFacade interface via Go structural typing.
// It looks up the type of a subcategory's parent category and returns it as a plain string,
// avoiding any type coupling between the category and transaction modules.
type SubcategoryFacade struct {
	getSubcategory GetSubcategoryUseCase
	getCategory    GetCategoryUseCase
}

// NewSubcategoryFacade creates a new SubcategoryFacade.
func NewSubcategoryFacade(getSub GetSubcategoryUseCase, getCat GetCategoryUseCase) *SubcategoryFacade {
	return &SubcategoryFacade{getSubcategory: getSub, getCategory: getCat}
}

// GetSubcategoryType returns the string type ("despesa", "receita", "transferencia") of the
// parent category for the given subcategory ID.
// This method signature satisfies transaction.SubcategoryFacade at the call site in main.go.
func (f *SubcategoryFacade) GetSubcategoryType(ctx context.Context, subcategoryID string) (string, error) {
	sub, err := f.getSubcategory.Execute(ctx, subcategoryID)
	if err != nil {
		return "", err
	}
	cat, err := f.getCategory.Execute(ctx, sub.CategoryID)
	if err != nil {
		return "", err
	}
	return string(cat.Type), nil
}
