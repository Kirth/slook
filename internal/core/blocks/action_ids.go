package blocks

// ApplyActionIDs copies legacy action ids -- captured from the raw
// WebSocket frame, since slack-go's AttachmentAction does not declare
// the field -- onto parsed attachments by position: ids[i][j] belongs to
// atts[i].Actions[j]. Missing ids leave ID empty; extra ids are ignored.
func ApplyActionIDs(atts []LegacyAttachment, ids [][]string) {
	for i := range atts {
		if i >= len(ids) {
			return
		}
		for j := range atts[i].Actions {
			if j >= len(ids[i]) {
				break
			}
			atts[i].Actions[j].ID = ids[i][j]
		}
	}
}
