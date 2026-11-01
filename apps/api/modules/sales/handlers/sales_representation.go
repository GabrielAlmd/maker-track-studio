package saleshandlers

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
)

func salesChannelDTO(channel salesmodels.SalesChannel) salesdto.SalesChannelDTO {
	return salesdto.SalesChannelDTO{
		ID:         channel.ID,
		Name:       channel.Name,
		Notes:      channel.Notes,
		IsArchived: channel.IsArchived,
		CreatedAt:  channel.CreatedAt,
		UpdatedAt:  channel.UpdatedAt,
	}
}

func salesOrderDTO(order salesmodels.SalesOrder, profitability salesdto.SalesOrderProfitabilityDTO) salesdto.SalesOrderDTO {
	items := make([]salesdto.SalesOrderItemDTO, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, salesdto.SalesOrderItemDTO{
			ID:                item.ID,
			SalesOrderID:      item.SalesOrderID,
			ModelID:           item.ModelID,
			ModelNameSnapshot: item.ModelNameSnapshot,
			Quantity:          item.Quantity,
			UnitPriceCents:    item.UnitPriceCents,
			CreatedAt:         item.CreatedAt,
			UpdatedAt:         item.UpdatedAt,
		})
	}
	result := salesdto.SalesOrderDTO{
		ID:                 order.ID,
		Reference:          order.Reference,
		OrderDate:          order.OrderDate,
		SalesChannelID:     order.SalesChannelID,
		FulfilmentStatus:   order.FulfilmentStatus,
		PaymentStatus:      order.PaymentStatus,
		PackagingCostCents: order.PackagingCostCents,
		ShippingCostCents:  order.ShippingCostCents,
		PlatformFeeCents:   order.PlatformFeeCents,
		OtherCostCents:     order.OtherCostCents,
		Notes:              order.Notes,
		Items:              items,
		Profitability:      profitability,
		CreatedAt:          order.CreatedAt,
		UpdatedAt:          order.UpdatedAt,
	}
	if order.SalesChannel != nil {
		channel := salesChannelDTO(*order.SalesChannel)
		result.SalesChannel = &channel
	}
	return result
}
