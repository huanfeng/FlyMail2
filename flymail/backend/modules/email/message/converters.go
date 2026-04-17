package message

import (
	"strconv"
	"strings"

	"flymail/modules/email/message/dto"
)

// ConvertEmailToListResponse converts an Email to EmailListResponse
func ConvertEmailToListResponse(email *Email) *dto.EmailListResponse {
	return &dto.EmailListResponse{
		EmailID:       email.EmailID,
		UID:           strconv.FormatUint(uint64(email.UID), 10),
		MessageID:     email.MessageID,
		AccountID:     email.AccountID,
		Subject:       email.Subject,
		From:          email.From,
		To:            email.To,
		CC:            email.CC,
		BCC:           email.BCC,
		Date:          email.Date,
		IsRead:        email.IsRead,
		IsStarred:     email.IsStarred,
		HasAttachment: len(email.Attachments) > 0,
		Size:          email.Size,
		Preview:       generatePreview(email.Body, 150),
		FolderName:    email.FolderName,
		FolderType:    email.FolderType,
		InternalDate:  email.Date, // Use Date as InternalDate
		CreatedAt:     email.CreatedAt,
	}
}

// ConvertEmailToDetailResponse converts an Email to EmailDetailResponse
func ConvertEmailToDetailResponse(email *Email) *dto.EmailDetailResponse {
	return &dto.EmailDetailResponse{
		EmailListResponse: *ConvertEmailToListResponse(email),
		TextBody:          email.Body,
		HTMLBody:          email.BodyHTML,
		Attachments:       convertAttachments(&email.Attachments),
		Headers:           nil, // TODO: Add headers support
		UpdatedAt:         email.UpdatedAt,
	}
}

// ConvertEmailsToListResponse converts a slice of Emails to EmailListResponses
func ConvertEmailsToListResponse(emails []*Email) []*dto.EmailListResponse {
	responses := make([]*dto.EmailListResponse, len(emails))
	for i, email := range emails {
		responses[i] = ConvertEmailToListResponse(email)
	}
	return responses
}

// generatePreview generates a preview from text body
func generatePreview(body string, maxLength int) string {
	// Remove extra whitespace
	preview := strings.TrimSpace(body)
	preview = strings.ReplaceAll(preview, "\n", " ")
	preview = strings.ReplaceAll(preview, "\r", " ")
	preview = strings.ReplaceAll(preview, "\t", " ")

	// Replace multiple spaces with single space
	for strings.Contains(preview, "  ") {
		preview = strings.ReplaceAll(preview, "  ", " ")
	}

	// Truncate if needed
	if len(preview) > maxLength {
		preview = preview[:maxLength] + "..."
	}

	return preview
}

// convertAttachments converts Attachments to AttachmentResponses
func convertAttachments(attachments *[]Attachment) []*dto.AttachmentResponse {
	if attachments == nil {
		return nil
	}
	responses := make([]*dto.AttachmentResponse, len(*attachments))
	for i, att := range *attachments {
		responses[i] = &dto.AttachmentResponse{
			AttachmentID: att.AttachmentID,
			Filename:     att.Filename,
			Size:         att.Size,
			ContentType:  att.ContentType,
			ContentID:    "",    // TODO: Add ContentID support
			IsInline:     false, // TODO: Add IsInline support
		}
	}
	return responses
}
