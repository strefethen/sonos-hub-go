package music

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMusicSetJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	set := MusicSet{
		SetID:           "set-123",
		Name:            "Morning Playlist",
		SelectionPolicy: string(SelectionPolicyRotation),
		CurrentIndex:    2,
		ItemCount:       5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Marshal
	data, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("failed to marshal MusicSet: %v", err)
	}

	// Unmarshal
	var decoded MusicSet
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal MusicSet: %v", err)
	}

	// Verify fields
	if decoded.SetID != set.SetID {
		t.Errorf("SetID mismatch: got %s, want %s", decoded.SetID, set.SetID)
	}
	if decoded.Name != set.Name {
		t.Errorf("Name mismatch: got %s, want %s", decoded.Name, set.Name)
	}
	if decoded.SelectionPolicy != set.SelectionPolicy {
		t.Errorf("SelectionPolicy mismatch: got %s, want %s", decoded.SelectionPolicy, set.SelectionPolicy)
	}
	if decoded.CurrentIndex != set.CurrentIndex {
		t.Errorf("CurrentIndex mismatch: got %d, want %d", decoded.CurrentIndex, set.CurrentIndex)
	}
	if decoded.ItemCount != set.ItemCount {
		t.Errorf("ItemCount mismatch: got %d, want %d", decoded.ItemCount, set.ItemCount)
	}
}

func TestMusicSetJSON_OmitEmptyItemCount(t *testing.T) {
	set := MusicSet{
		SetID:           "set-123",
		Name:            "Test",
		SelectionPolicy: string(SelectionPolicyShuffle),
		CurrentIndex:    0,
		ItemCount:       0, // Should be omitted
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	data, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("failed to marshal MusicSet: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, exists := raw["item_count"]; exists {
		t.Error("item_count should be omitted when zero")
	}
}

func TestSetItemJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	logoURL := "https://example.com/logo.png"
	serviceName := "Spotify"
	contentJSON := `{"title":"Test Song"}`

	item := SetItem{
		SetID:           "set-123",
		SonosFavoriteID: "fav-456",
		Position:        0,
		ServiceLogoURL:  &logoURL,
		ServiceName:     &serviceName,
		ContentType:     string(ContentTypeSonosFavorite),
		ContentJSON:     &contentJSON,
		AddedAt:         now,
	}

	// Marshal
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal SetItem: %v", err)
	}

	// Unmarshal
	var decoded SetItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SetItem: %v", err)
	}

	// Verify fields
	if decoded.SetID != item.SetID {
		t.Errorf("SetID mismatch: got %s, want %s", decoded.SetID, item.SetID)
	}
	if decoded.SonosFavoriteID != item.SonosFavoriteID {
		t.Errorf("SonosFavoriteID mismatch: got %s, want %s", decoded.SonosFavoriteID, item.SonosFavoriteID)
	}
	if decoded.Position != item.Position {
		t.Errorf("Position mismatch: got %d, want %d", decoded.Position, item.Position)
	}
	if decoded.ServiceLogoURL == nil || *decoded.ServiceLogoURL != logoURL {
		t.Errorf("ServiceLogoURL mismatch")
	}
	if decoded.ServiceName == nil || *decoded.ServiceName != serviceName {
		t.Errorf("ServiceName mismatch")
	}
	if decoded.ContentType != item.ContentType {
		t.Errorf("ContentType mismatch: got %s, want %s", decoded.ContentType, item.ContentType)
	}
	if decoded.ContentJSON == nil || *decoded.ContentJSON != contentJSON {
		t.Errorf("ContentJSON mismatch")
	}
}

func TestSetItemJSON_NilOptionalFields(t *testing.T) {
	item := SetItem{
		SetID:           "set-123",
		SonosFavoriteID: "fav-456",
		Position:        0,
		ContentType:     string(ContentTypeDirect),
		AddedAt:         time.Now(),
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal SetItem: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Optional fields should be omitted
	if _, exists := raw["service_logo_url"]; exists {
		t.Error("service_logo_url should be omitted when nil")
	}
	if _, exists := raw["service_name"]; exists {
		t.Error("service_name should be omitted when nil")
	}
	if _, exists := raw["content_json"]; exists {
		t.Error("content_json should be omitted when nil")
	}
}

func TestPlayHistoryJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	setID := "set-123"
	routineID := "routine-789"

	history := PlayHistory{
		ID:              42,
		SonosFavoriteID: "fav-456",
		SetID:           &setID,
		RoutineID:       &routineID,
		PlayedAt:        now,
	}

	// Marshal
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatalf("failed to marshal PlayHistory: %v", err)
	}

	// Unmarshal
	var decoded PlayHistory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PlayHistory: %v", err)
	}

	// Verify fields
	if decoded.ID != history.ID {
		t.Errorf("ID mismatch: got %d, want %d", decoded.ID, history.ID)
	}
	if decoded.SonosFavoriteID != history.SonosFavoriteID {
		t.Errorf("SonosFavoriteID mismatch: got %s, want %s", decoded.SonosFavoriteID, history.SonosFavoriteID)
	}
	if decoded.SetID == nil || *decoded.SetID != setID {
		t.Errorf("SetID mismatch")
	}
	if decoded.RoutineID == nil || *decoded.RoutineID != routineID {
		t.Errorf("RoutineID mismatch")
	}
}

func TestPlayHistoryJSON_NilOptionalFields(t *testing.T) {
	history := PlayHistory{
		ID:              1,
		SonosFavoriteID: "fav-123",
		PlayedAt:        time.Now(),
	}

	data, err := json.Marshal(history)
	if err != nil {
		t.Fatalf("failed to marshal PlayHistory: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Optional fields should be omitted
	if _, exists := raw["set_id"]; exists {
		t.Error("set_id should be omitted when nil")
	}
	if _, exists := raw["routine_id"]; exists {
		t.Error("routine_id should be omitted when nil")
	}
}

func TestContentMetadataJSON(t *testing.T) {
	metadata := ContentMetadata{
		Title:        "Test Song",
		Artist:       "Test Artist",
		Album:        "Test Album",
		ArtworkURL:   "https://example.com/artwork.jpg",
		URI:          "x-sonos-spotify:spotify:track:123",
		Metadata:     "<DIDL-Lite>...</DIDL-Lite>",
		AppleMusicID: "am-123",
	}

	// Marshal
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("failed to marshal ContentMetadata: %v", err)
	}

	// Unmarshal
	var decoded ContentMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ContentMetadata: %v", err)
	}

	// Verify fields
	if decoded.Title != metadata.Title {
		t.Errorf("Title mismatch: got %s, want %s", decoded.Title, metadata.Title)
	}
	if decoded.Artist != metadata.Artist {
		t.Errorf("Artist mismatch: got %s, want %s", decoded.Artist, metadata.Artist)
	}
	if decoded.Album != metadata.Album {
		t.Errorf("Album mismatch: got %s, want %s", decoded.Album, metadata.Album)
	}
	if decoded.ArtworkURL != metadata.ArtworkURL {
		t.Errorf("ArtworkURL mismatch: got %s, want %s", decoded.ArtworkURL, metadata.ArtworkURL)
	}
	if decoded.URI != metadata.URI {
		t.Errorf("URI mismatch: got %s, want %s", decoded.URI, metadata.URI)
	}
	if decoded.Metadata != metadata.Metadata {
		t.Errorf("Metadata mismatch: got %s, want %s", decoded.Metadata, metadata.Metadata)
	}
	if decoded.AppleMusicID != metadata.AppleMusicID {
		t.Errorf("AppleMusicID mismatch: got %s, want %s", decoded.AppleMusicID, metadata.AppleMusicID)
	}
}

func TestContentMetadataJSON_OmitEmptyFields(t *testing.T) {
	metadata := ContentMetadata{
		Title: "Test Song",
		// All other fields empty
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("failed to marshal ContentMetadata: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Only title should be present
	if _, exists := raw["title"]; !exists {
		t.Error("title should be present")
	}

	// Empty fields should be omitted
	emptyFields := []string{"artist", "album", "artwork_url", "uri", "metadata", "apple_music_id"}
	for _, field := range emptyFields {
		if _, exists := raw[field]; exists {
			t.Errorf("%s should be omitted when empty", field)
		}
	}
}

func TestSelectionResultJSON(t *testing.T) {
	item := &SetItem{
		SetID:           "set-123",
		SonosFavoriteID: "fav-456",
		Position:        2,
		ContentType:     string(ContentTypeSonosFavorite),
		AddedAt:         time.Now(),
	}

	result := SelectionResult{
		Item:        item,
		NextIndex:   3,
		WasShuffled: true,
	}

	// Marshal
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SelectionResult: %v", err)
	}

	// Unmarshal
	var decoded SelectionResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SelectionResult: %v", err)
	}

	// Verify fields
	if decoded.Item == nil {
		t.Fatal("Item should not be nil")
	}
	if decoded.Item.SonosFavoriteID != item.SonosFavoriteID {
		t.Errorf("Item.SonosFavoriteID mismatch")
	}
	if decoded.NextIndex != result.NextIndex {
		t.Errorf("NextIndex mismatch: got %d, want %d", decoded.NextIndex, result.NextIndex)
	}
	if decoded.WasShuffled != result.WasShuffled {
		t.Errorf("WasShuffled mismatch: got %v, want %v", decoded.WasShuffled, result.WasShuffled)
	}
}

func TestCreateSetInputJSON(t *testing.T) {
	input := CreateSetInput{
		Name:            "New Playlist",
		SelectionPolicy: string(SelectionPolicyShuffle),
	}

	// Marshal
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal CreateSetInput: %v", err)
	}

	// Unmarshal
	var decoded CreateSetInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal CreateSetInput: %v", err)
	}

	if decoded.Name != input.Name {
		t.Errorf("Name mismatch: got %s, want %s", decoded.Name, input.Name)
	}
	if decoded.SelectionPolicy != input.SelectionPolicy {
		t.Errorf("SelectionPolicy mismatch: got %s, want %s", decoded.SelectionPolicy, input.SelectionPolicy)
	}
}

func TestUpdateSetInputJSON(t *testing.T) {
	name := "Updated Name"
	policy := string(SelectionPolicyRotation)

	input := UpdateSetInput{
		Name:            &name,
		SelectionPolicy: &policy,
	}

	// Marshal
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal UpdateSetInput: %v", err)
	}

	// Unmarshal
	var decoded UpdateSetInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal UpdateSetInput: %v", err)
	}

	if decoded.Name == nil || *decoded.Name != name {
		t.Errorf("Name mismatch")
	}
	if decoded.SelectionPolicy == nil || *decoded.SelectionPolicy != policy {
		t.Errorf("SelectionPolicy mismatch")
	}
}

func TestUpdateSetInputJSON_OmitNilFields(t *testing.T) {
	input := UpdateSetInput{
		Name: nil,
		// SelectionPolicy intentionally nil
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal UpdateSetInput: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, exists := raw["name"]; exists {
		t.Error("name should be omitted when nil")
	}
	if _, exists := raw["selection_policy"]; exists {
		t.Error("selection_policy should be omitted when nil")
	}
}

func TestAddItemInputJSON(t *testing.T) {
	logoURL := "https://example.com/logo.png"
	serviceName := "Apple Music"
	contentJSON := `{"apple_music_id":"am-123"}`

	input := AddItemInput{
		SonosFavoriteID: "fav-123",
		ServiceLogoURL:  &logoURL,
		ServiceName:     &serviceName,
		ContentType:     string(ContentTypeAppleMusic),
		ContentJSON:     &contentJSON,
	}

	// Marshal
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal AddItemInput: %v", err)
	}

	// Unmarshal
	var decoded AddItemInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal AddItemInput: %v", err)
	}

	if decoded.SonosFavoriteID != input.SonosFavoriteID {
		t.Errorf("SonosFavoriteID mismatch")
	}
	if decoded.ServiceLogoURL == nil || *decoded.ServiceLogoURL != logoURL {
		t.Errorf("ServiceLogoURL mismatch")
	}
	if decoded.ServiceName == nil || *decoded.ServiceName != serviceName {
		t.Errorf("ServiceName mismatch")
	}
	if decoded.ContentType != input.ContentType {
		t.Errorf("ContentType mismatch")
	}
	if decoded.ContentJSON == nil || *decoded.ContentJSON != contentJSON {
		t.Errorf("ContentJSON mismatch")
	}
}

func TestReorderItemsInputJSON(t *testing.T) {
	input := ReorderItemsInput{
		Items: []string{"fav-3", "fav-1", "fav-2"},
	}

	// Marshal
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal ReorderItemsInput: %v", err)
	}

	// Unmarshal
	var decoded ReorderItemsInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ReorderItemsInput: %v", err)
	}

	if len(decoded.Items) != len(input.Items) {
		t.Errorf("Items length mismatch: got %d, want %d", len(decoded.Items), len(input.Items))
	}
	for i, item := range input.Items {
		if decoded.Items[i] != item {
			t.Errorf("Items[%d] mismatch: got %s, want %s", i, decoded.Items[i], item)
		}
	}
}

func TestPlaySetInputJSON(t *testing.T) {
	input := PlaySetInput{
		UDN:       "RINCON_123",
		QueueMode: string(QueueModeReplaceAndPlay),
	}

	// Marshal
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal PlaySetInput: %v", err)
	}

	// Unmarshal
	var decoded PlaySetInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PlaySetInput: %v", err)
	}

	if decoded.UDN != input.UDN {
		t.Errorf("UDN mismatch: got %s, want %s", decoded.UDN, input.UDN)
	}
	if decoded.QueueMode != input.QueueMode {
		t.Errorf("QueueMode mismatch: got %s, want %s", decoded.QueueMode, input.QueueMode)
	}
}

func TestPlaySetInputJSON_OmitEmptyQueueMode(t *testing.T) {
	input := PlaySetInput{
		UDN: "RINCON_123",
		// QueueMode intentionally empty
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal PlaySetInput: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, exists := raw["queue_mode"]; exists {
		t.Error("queue_mode should be omitted when empty")
	}
}

func TestSelectItemInputJSON(t *testing.T) {
	windowMinutes := 60

	input := SelectItemInput{
		NoRepeatWindowMinutes: &windowMinutes,
	}

	// Marshal
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal SelectItemInput: %v", err)
	}

	// Unmarshal
	var decoded SelectItemInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SelectItemInput: %v", err)
	}

	if decoded.NoRepeatWindowMinutes == nil || *decoded.NoRepeatWindowMinutes != windowMinutes {
		t.Errorf("NoRepeatWindowMinutes mismatch")
	}
}

func TestSelectItemInputJSON_OmitNilField(t *testing.T) {
	input := SelectItemInput{
		NoRepeatWindowMinutes: nil,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal SelectItemInput: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, exists := raw["no_repeat_window_minutes"]; exists {
		t.Error("no_repeat_window_minutes should be omitted when nil")
	}
}

func TestSelectionPolicyConstants(t *testing.T) {
	if SelectionPolicyRotation != "ROTATION" {
		t.Errorf("SelectionPolicyRotation should be ROTATION, got %s", SelectionPolicyRotation)
	}
	if SelectionPolicyShuffle != "SHUFFLE" {
		t.Errorf("SelectionPolicyShuffle should be SHUFFLE, got %s", SelectionPolicyShuffle)
	}
}

func TestContentTypeConstants(t *testing.T) {
	if ContentTypeSonosFavorite != "sonos_favorite" {
		t.Errorf("ContentTypeSonosFavorite should be sonos_favorite, got %s", ContentTypeSonosFavorite)
	}
	if ContentTypeAppleMusic != "apple_music" {
		t.Errorf("ContentTypeAppleMusic should be apple_music, got %s", ContentTypeAppleMusic)
	}
	if ContentTypeDirect != "direct" {
		t.Errorf("ContentTypeDirect should be direct, got %s", ContentTypeDirect)
	}
}

func TestQueueModeConstants(t *testing.T) {
	if QueueModeReplaceAndPlay != "REPLACE_AND_PLAY" {
		t.Errorf("QueueModeReplaceAndPlay should be REPLACE_AND_PLAY, got %s", QueueModeReplaceAndPlay)
	}
	if QueueModePlayNext != "PLAY_NEXT" {
		t.Errorf("QueueModePlayNext should be PLAY_NEXT, got %s", QueueModePlayNext)
	}
	if QueueModeAddToEnd != "ADD_TO_END" {
		t.Errorf("QueueModeAddToEnd should be ADD_TO_END, got %s", QueueModeAddToEnd)
	}
	if QueueModeQueueOnly != "QUEUE_ONLY" {
		t.Errorf("QueueModeQueueOnly should be QUEUE_ONLY, got %s", QueueModeQueueOnly)
	}
}
