package operators

import (
	"fmt"

	"github.com/flip-bills/backend/internal/models"
)

// buildDefaultSeatLayout generates a standard seat map for a given bus.
// rows = number of seat rows, seatsPerRow = seats across (typically 4 for Nigerian buses).
// This drives the PRD's "interactive live layout map" in Click 2 of the checkout.
func buildDefaultSeatLayout(rows, seatsPerRow int) []models.SeatRow {
	letters := []string{"A", "B", "C", "D", "E"}
	layout := make([]models.SeatRow, rows)

	for r := 0; r < rows; r++ {
		row := models.SeatRow{Row: r + 1}
		for s := 0; s < seatsPerRow && s < len(letters); s++ {
			row.Seats = append(row.Seats, models.Seat{
				Number:    fmt.Sprintf("%d%s", r+1, letters[s]),
				Available: true,
				Class:     "standard",
			})
		}
		layout[r] = row
	}
	return layout
}
