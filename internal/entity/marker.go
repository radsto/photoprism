package entity

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/dustin/go-humanize/english"

	"github.com/jinzhu/gorm"

	"github.com/photoprism/photoprism/internal/crop"
	"github.com/photoprism/photoprism/internal/face"
	"github.com/photoprism/photoprism/internal/form"
	"github.com/photoprism/photoprism/pkg/rnd"
	"github.com/photoprism/photoprism/pkg/txt"
)

const (
	MarkerUnknown = ""
	MarkerFace    = "face"  // MarkerType for faces (implemented).
	MarkerLabel   = "label" // MarkerType for labels (todo).
)

// Marker represents an image marker point.
type Marker struct {
	MarkerUID      string          `gorm:"type:VARBINARY(42);primary_key;auto_increment:false;" json:"UID" yaml:"UID"`
	FileUID        string          `gorm:"type:VARBINARY(42);index;default:'';" json:"FileUID" yaml:"FileUID"`
	MarkerType     string          `gorm:"type:VARBINARY(8);default:'';" json:"Type" yaml:"Type"`
	MarkerSrc      string          `gorm:"type:VARBINARY(8);default:'';" json:"Src" yaml:"Src,omitempty"`
	MarkerName     string          `gorm:"type:VARCHAR(160);" json:"Name" yaml:"Name,omitempty"`
	MarkerReview   bool            `json:"Review" yaml:"Review,omitempty"`
	MarkerInvalid  bool            `json:"Invalid" yaml:"Invalid,omitempty"`
	SubjUID        string          `gorm:"type:VARBINARY(42);index:idx_markers_subj_uid_src;" json:"SubjUID" yaml:"SubjUID,omitempty"`
	SubjSrc        string          `gorm:"type:VARBINARY(8);index:idx_markers_subj_uid_src;default:'';" json:"SubjSrc" yaml:"SubjSrc,omitempty"`
	subject        *Subject        `gorm:"foreignkey:SubjUID;association_foreignkey:SubjUID;association_autoupdate:false;association_autocreate:false;association_save_reference:false"`
	FaceID         string          `gorm:"type:VARBINARY(42);index;" json:"FaceID" yaml:"FaceID,omitempty"`
	FaceDist       float64         `gorm:"default:-1;" json:"FaceDist" yaml:"FaceDist,omitempty"`
	face           *Face           `gorm:"foreignkey:FaceID;association_foreignkey:ID;association_autoupdate:false;association_autocreate:false;association_save_reference:false"`
	EmbeddingsJSON json.RawMessage `gorm:"type:MEDIUMBLOB;" json:"-" yaml:"EmbeddingsJSON,omitempty"`
	embeddings     face.Embeddings `gorm:"-"`
	LandmarksJSON  json.RawMessage `gorm:"type:MEDIUMBLOB;" json:"-" yaml:"LandmarksJSON,omitempty"`
	X              float32         `gorm:"type:FLOAT;" json:"X" yaml:"X,omitempty"`
	Y              float32         `gorm:"type:FLOAT;" json:"Y" yaml:"Y,omitempty"`
	W              float32         `gorm:"type:FLOAT;" json:"W" yaml:"W,omitempty"`
	H              float32         `gorm:"type:FLOAT;" json:"H" yaml:"H,omitempty"`
	Q              int             `json:"Q" yaml:"Q,omitempty"`
	Size           int             `gorm:"default:-1;" json:"Size" yaml:"Size,omitempty"`
	Score          int             `gorm:"type:SMALLINT;" json:"Score" yaml:"Score,omitempty"`
	Thumb          string          `gorm:"type:VARBINARY(128);index;default:'';" json:"Thumb" yaml:"Thumb,omitempty"`
	MatchedAt      *time.Time      `sql:"index" json:"MatchedAt" yaml:"MatchedAt,omitempty"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName returns the entity database table name.
func (Marker) TableName() string {
	return "markers"
}

// BeforeCreate creates a random UID if needed before inserting a new row to the database.
func (m *Marker) BeforeCreate(scope *gorm.Scope) error {
	if rnd.IsUID(m.MarkerUID, 'm') {
		return nil
	}

	return scope.SetColumn("MarkerUID", rnd.PPID('m'))
}

// NewMarker creates a new entity.
func NewMarker(file File, area crop.Area, subjUID, markerSrc, markerType string, size, score int) *Marker {
	if file.FileHash == "" {
		log.Errorf("markers: file hash is empty - you might have found a bug")
		return nil
	}

	m := &Marker{
		FileUID:       file.FileUID,
		MarkerSrc:     markerSrc,
		MarkerType:    markerType,
		MarkerReview:  score < 30,
		MarkerInvalid: false,
		SubjUID:       subjUID,
		FaceDist:      -1,
		X:             area.X,
		Y:             area.Y,
		W:             area.W,
		H:             area.H,
		Q:             int(float32(math.Log(float64(score))) * float32(size) * area.W),
		Size:          size,
		Score:         score,
		Thumb:         area.Thumb(file.FileHash),
		MatchedAt:     nil,
	}

	return m
}

// NewFaceMarker creates a new entity.
func NewFaceMarker(f face.Face, file File, subjUID string) *Marker {
	m := NewMarker(file, f.CropArea(), subjUID, SrcImage, MarkerFace, f.Size(), f.Score)

	// Failed creating new marker?
	if m == nil {
		return nil
	}

	m.SetEmbeddings(f.Embeddings)
	m.LandmarksJSON = f.RelativeLandmarksJSON()

	return m
}

// SetEmbeddings assigns new face emebddings to the marker.
func (m *Marker) SetEmbeddings(e face.Embeddings) {
	m.embeddings = e
	m.EmbeddingsJSON = e.JSON()
}

// UpdateFile sets the file uid and thumb and updates the index if the marker already exists.
func (m *Marker) UpdateFile(file *File) (updated bool) {
	if file.FileUID != "" && m.FileUID != file.FileUID {
		m.FileUID = file.FileUID
		updated = true
	}

	if file.FileHash != "" && !strings.HasPrefix(m.Thumb, file.FileHash) {
		m.Thumb = crop.NewArea("crop", m.X, m.Y, m.W, m.H).Thumb(file.FileHash)
		updated = true
	}

	if !updated || m.MarkerUID == "" {
		return false
	} else if res := UnscopedDb().Model(m).UpdateColumns(Values{"file_uid": m.FileUID, "thumb": m.Thumb}); res.Error != nil {
		log.Errorf("marker %s: %s (set file)", m.MarkerUID, res.Error)
		return false
	} else {
		return true
	}
}

// Updates multiple columns in the database.
func (m *Marker) Updates(values interface{}) error {
	return UnscopedDb().Model(m).Updates(values).Error
}

// Update updates a column in the database.
func (m *Marker) Update(attr string, value interface{}) error {
	return UnscopedDb().Model(m).Update(attr, value).Error
}

// SetName changes the marker name.
func (m *Marker) SetName(name, src string) (changed bool, err error) {
	if src == SrcAuto || SrcPriority[src] < SrcPriority[m.SubjSrc] {
		return false, nil
	}

	name = txt.NormalizeName(name)

	if name == "" {
		return false, nil
	}

	if m.MarkerName == name {
		// Name didn't change.
		return false, nil
	}

	m.SubjSrc = src
	m.MarkerName = name

	return true, m.SyncSubject(true)
}

// SaveForm updates the entity using form data and stores it in the database.
func (m *Marker) SaveForm(f form.Marker) (changed bool, err error) {
	if m.MarkerInvalid != f.MarkerInvalid {
		m.MarkerInvalid = f.MarkerInvalid
		changed = true
	}

	if m.MarkerReview != f.MarkerReview {
		m.MarkerReview = f.MarkerReview
		changed = true
	}

	if nameChanged, err := m.SetName(f.MarkerName, f.SubjSrc); err != nil {
		return changed, err
	} else if nameChanged {
		changed = true
	}

	if changed {
		return changed, m.Save()
	}

	return changed, nil
}

// HasFace tests if the marker already has the best matching face.
func (m *Marker) HasFace(f *Face, dist float64) bool {
	if m.FaceID == "" {
		return false
	} else if f == nil {
		return m.FaceID != ""
	} else if m.FaceID == f.ID {
		return m.FaceID != ""
	} else if m.FaceDist < 0 {
		return false
	} else if dist < 0 {
		return true
	}

	return m.FaceDist <= dist
}

// SetFace sets a new face for this marker.
func (m *Marker) SetFace(f *Face, dist float64) (updated bool, err error) {
	if f == nil {
		return false, fmt.Errorf("face is nil")
	}

	if m.MarkerType != MarkerFace {
		return false, fmt.Errorf("not a face marker")
	}

	// Any reason we don't want to set a new face for this marker?
	if m.SubjSrc == SrcAuto || f.SubjUID == "" || m.SubjUID == "" || f.SubjUID == m.SubjUID {
		// Don't skip if subject wasn't set manually, or subjects match.
	} else if reported, err := f.ResolveCollision(m.Embeddings()); err != nil {
		return false, err
	} else if reported {
		log.Warnf("marker %s: face %s has ambiguous subjects %s <> %s, subject source %s", txt.Quote(m.MarkerUID), txt.Quote(f.ID), txt.Quote(m.SubjUID), txt.Quote(f.SubjUID), SrcString(m.SubjSrc))
		return false, nil
	} else {
		return false, nil
	}

	// Update face with known subject from marker?
	if m.SubjSrc == SrcAuto || m.SubjUID == "" || f.SubjUID != "" {
		// Don't update if face has a known subject, or marker subject is unknown.
	} else if err = f.SetSubjectUID(m.SubjUID); err != nil {
		return false, err
	}

	// Set face.
	m.face = f

	// Skip update if the same face is already set.
	if m.SubjUID == f.SubjUID && m.FaceID == f.ID {
		// Update matching timestamp.
		m.MatchedAt = TimePointer()
		return false, m.Updates(Values{"MatchedAt": m.MatchedAt})
	}

	// Remember current values for comparison.
	faceID := m.FaceID
	subjUID := m.SubjUID
	subjSrc := m.SubjSrc

	m.FaceID = f.ID
	m.FaceDist = dist

	if m.FaceDist < 0 {
		faceEmbedding := f.Embedding()

		// Calculate the smallest distance to embeddings.
		for _, e := range m.Embeddings() {
			if len(e) != len(faceEmbedding) {
				continue
			}

			if d := e.Distance(faceEmbedding); d < m.FaceDist || m.FaceDist < 0 {
				m.FaceDist = d
			}
		}
	}

	if f.SubjUID != "" {
		m.SubjUID = f.SubjUID
	}

	if err = m.SyncSubject(false); err != nil {
		return false, err
	}

	// Update face subject?
	if m.SubjSrc == SrcAuto || m.SubjUID == "" || f.SubjUID == m.SubjUID {
		// Not needed.
	} else if err = f.SetSubjectUID(m.SubjUID); err != nil {
		return false, err
	}

	updated = m.FaceID != faceID || m.SubjUID != subjUID || m.SubjSrc != subjSrc

	// Update matching timestamp.
	m.MatchedAt = TimePointer()

	if err := m.Updates(Values{"FaceID": m.FaceID, "FaceDist": m.FaceDist, "SubjUID": m.SubjUID, "SubjSrc": m.SubjSrc, "MarkerReview": false, "MatchedAt": m.MatchedAt}); err != nil {
		return false, err
	} else if !updated {
		return false, nil
	}

	return true, m.RefreshPhotos()
}

// SyncSubject maintains the marker subject relationship.
func (m *Marker) SyncSubject(updateRelated bool) (err error) {
	// Face marker? If not, return.
	if m.MarkerType != MarkerFace {
		return nil
	}

	subj := m.Subject()

	if subj == nil || m.SubjSrc == SrcAuto {
		return nil
	}

	// Update subject with marker name?
	if m.MarkerName == "" || subj.SubjName == m.MarkerName {
		// Do nothing.
	} else if subj, err = subj.UpdateName(m.MarkerName); err != nil {
		return err
	} else if subj != nil {
		// Update subject fields in case it was merged.
		m.subject = subj
		m.SubjUID = subj.SubjUID
		m.MarkerName = subj.SubjName
	}

	// Create known face for subject?
	if m.FaceID != "" {
		// Do nothing.
	} else if f := m.Face(); f != nil {
		m.FaceID = f.ID
	}

	// Update related markers?
	if m.FaceID == "" || m.SubjUID == "" {
		// Do nothing.
	} else if res := Db().Model(&Face{}).Where("id = ? AND subj_uid = ''", m.FaceID).UpdateColumn("subj_uid", m.SubjUID); res.Error != nil {
		return fmt.Errorf("%s (update known face)", err)
	} else if !updateRelated {
		return nil
	} else if err := Db().Model(&Marker{}).
		Where("marker_uid <> ?", m.MarkerUID).
		Where("face_id = ?", m.FaceID).
		Where("subj_src = ?", SrcAuto).
		Where("subj_uid <> ?", m.SubjUID).
		UpdateColumns(Values{"subj_uid": m.SubjUID, "subj_src": SrcAuto, "marker_review": false}).Error; err != nil {
		return fmt.Errorf("%s (update related markers)", err)
	} else if res.RowsAffected > 0 && m.face != nil {
		log.Debugf("markers: matched %s with %s", subj.SubjName, m.FaceID)
		return m.face.RefreshPhotos()
	}

	return nil
}

// InvalidArea tests if the marker area is invalid or out of range.
func (m *Marker) InvalidArea() error {
	if m.MarkerType != MarkerFace {
		return nil
	}

	// Ok?
	if false == (m.X > 1 || m.Y > 1 || m.X < 0 || m.Y < 0 || m.W <= 0 || m.H <= 0 || m.W > 1 || m.H > 1) {
		return nil
	}

	return fmt.Errorf("invalid %s crop area x=%d%% y=%d%% w=%d%% h=%d%%", TypeString(m.MarkerType), int(m.X*100), int(m.Y*100), int(m.W*100), int(m.H*100))
}

// Save updates the existing or inserts a new row.
func (m *Marker) Save() error {
	if err := m.InvalidArea(); err != nil {
		return err
	}

	return Db().Save(m).Error
}

// Create inserts a new row to the database.
func (m *Marker) Create() error {
	if err := m.InvalidArea(); err != nil {
		return err
	}

	return Db().Create(m).Error
}

// Embeddings returns parsed marker embeddings.
func (m *Marker) Embeddings() face.Embeddings {
	if len(m.EmbeddingsJSON) == 0 {
		return face.Embeddings{}
	} else if len(m.embeddings) > 0 {
		return m.embeddings
	} else if err := json.Unmarshal(m.EmbeddingsJSON, &m.embeddings); err != nil {
		log.Errorf("markers: %s while parsing embeddings json", err)
	}

	return m.embeddings
}

// SubjectName returns the matching subject's name.
func (m *Marker) SubjectName() string {
	if m.MarkerName != "" {
		return m.MarkerName
	} else if s := m.Subject(); s != nil {
		return s.SubjName
	}

	return ""
}

// Subject returns the matching subject or nil.
func (m *Marker) Subject() (subj *Subject) {
	if m.subject != nil {
		if m.SubjUID == m.subject.SubjUID {
			return m.subject
		}
	}

	// Create subject?
	if m.SubjSrc != SrcAuto && m.MarkerName != "" && m.SubjUID == "" {
		if subj = NewSubject(m.MarkerName, SubjPerson, m.SubjSrc); subj == nil {
			log.Errorf("marker %s: invalid subject %s", txt.Quote(m.MarkerUID), txt.Quote(m.MarkerName))
			return nil
		} else if subj = FirstOrCreateSubject(subj); subj == nil {
			log.Debugf("marker %s: invalid subject %s", txt.Quote(m.MarkerUID), txt.Quote(m.MarkerName))
			return nil
		} else {
			m.subject = subj
			m.SubjUID = subj.SubjUID
		}

		return m.subject
	}

	m.subject = FindSubject(m.SubjUID)

	return m.subject
}

// ClearSubject removes an existing subject association, and reports a collision.
func (m *Marker) ClearSubject(src string) error {
	// Find the matching face.
	if m.face == nil {
		m.face = FindFace(m.FaceID)
	}

	defer func() {
		// Find and (soft) delete unused subjects.
		start := time.Now()
		if count, err := DeleteOrphanPeople(); err != nil {
			log.Errorf("marker %s: %s while removing unused subjects [%s]", txt.Quote(m.MarkerUID), err, time.Since(start))
		} else if count > 0 {
			log.Debugf("marker %s: removed %s [%s]", txt.Quote(m.MarkerUID), english.Plural(count, "person", "people"), time.Since(start))
		}
	}()

	// Update index & resolve collisions.
	if err := m.Updates(Values{"MarkerName": "", "FaceID": "", "FaceDist": -1.0, "SubjUID": "", "SubjSrc": src}); err != nil {
		return err
	} else if m.face == nil {
		m.subject = nil
		return nil
	} else if resolved, err := m.face.ResolveCollision(m.Embeddings()); err != nil {
		return err
	} else if resolved {
		log.Debugf("marker %s: resolved ambiguous subjects for face %s", txt.Quote(m.MarkerUID), txt.Quote(m.face.ID))
	}

	// Clear references.
	m.face = nil
	m.subject = nil

	return nil
}

// Face returns a matching face entity if possible.
func (m *Marker) Face() (f *Face) {
	if m.MarkerUID == "" {
		log.Debugf("markers: can't find face when uid is empty")
		return nil
	}

	if m.face != nil {
		if m.FaceID == m.face.ID {
			return m.face
		}
	}

	// Add face if size
	if m.SubjSrc != SrcAuto && m.FaceID == "" {
		if m.Size < face.ClusterSizeThreshold || m.Score < face.ClusterScoreThreshold {
			log.Debugf("marker %s: skipped adding face due to low-quality (size %d, score %d)", txt.Quote(m.MarkerUID), m.Size, m.Score)
			return nil
		} else if emb := m.Embeddings(); emb.Empty() {
			log.Warnf("marker %s: found no face embeddings", txt.Quote(m.MarkerUID))
			return nil
		} else if f = NewFace(m.SubjUID, m.SubjSrc, emb); f == nil {
			log.Warnf("marker %s: failed assigning face", txt.Quote(m.MarkerUID))
			return nil
		} else if f.Unsuitable() {
			log.Infof("marker %s: face %s is unsuitable for clustering and matching", txt.Quote(m.MarkerUID), f.ID)
		} else if f = FirstOrCreateFace(f); f == nil {
			log.Warnf("marker %s: failed assigning face", txt.Quote(m.MarkerUID))
			return nil
		} else if err := f.MatchMarkers(Faceless); err != nil {
			log.Errorf("marker %s: %s while matching with faces", txt.Quote(m.MarkerUID), err)
		}

		m.face = f
		m.FaceID = f.ID
		m.FaceDist = 0
	} else {
		m.face = FindFace(m.FaceID)
	}

	return m.face
}

// ClearFace removes an existing face association.
func (m *Marker) ClearFace() (updated bool, err error) {
	if m.FaceID == "" {
		return false, m.Matched()
	}

	updated = true

	// Remove face references.
	m.face = nil
	m.FaceID = ""
	m.MatchedAt = TimePointer()

	// Remove subject if set automatically.
	if m.SubjSrc == SrcAuto {
		m.SubjUID = ""
		err = m.Updates(Values{"FaceID": "", "FaceDist": -1.0, "SubjUID": "", "MatchedAt": m.MatchedAt})
	} else {
		err = m.Updates(Values{"FaceID": "", "FaceDist": -1.0, "MatchedAt": m.MatchedAt})
	}

	return updated, m.RefreshPhotos()
}

// RefreshPhotos flags related photos for metadata maintenance.
func (m *Marker) RefreshPhotos() error {
	if m.MarkerUID == "" {
		return fmt.Errorf("empty marker uid")
	}

	return UnscopedDb().Exec(`UPDATE photos SET checked_at = NULL WHERE id IN
		(SELECT f.photo_id FROM files f JOIN ? m ON m.file_uid = f.file_uid WHERE m.marker_uid = ? GROUP BY f.photo_id)`,
		gorm.Expr(Marker{}.TableName()), m.MarkerUID).Error
}

// Matched updates the match timestamp.
func (m *Marker) Matched() error {
	m.MatchedAt = TimePointer()
	return UnscopedDb().Model(m).UpdateColumns(Values{"MatchedAt": m.MatchedAt}).Error
}

// Top returns the top Y coordinate as float64.
func (m *Marker) Top() float64 {
	return float64(m.Y)
}

// Left returns the left X coordinate as float64.
func (m *Marker) Left() float64 {
	return float64(m.X)
}

// Right returns the right X coordinate as float64.
func (m *Marker) Right() float64 {
	return float64(m.X + m.W)
}

// Bottom returns the bottom Y coordinate as float64.
func (m *Marker) Bottom() float64 {
	return float64(m.Y + m.H)
}

// Surface returns the surface area.
func (m *Marker) Surface() float64 {
	return float64(m.W * m.H)
}

// SurfaceRatio returns the surface ratio.
func (m *Marker) SurfaceRatio(area float64) float64 {
	if area <= 0 {
		return 0
	}

	if s := m.Surface(); s <= 0 {
		return 0
	} else if area > s {
		return s / area
	} else {
		return area / s
	}
}

// Overlap calculates the overlap of two markers.
func (m *Marker) Overlap(marker Marker) (x, y float64) {
	x = math.Max(0, math.Min(m.Right(), marker.Right())-math.Max(m.Left(), marker.Left()))
	y = math.Max(0, math.Min(m.Bottom(), marker.Bottom())-math.Max(m.Top(), marker.Top()))

	return x, y
}

// OverlapArea calculates the overlap area of two markers.
func (m *Marker) OverlapArea(marker Marker) (area float64) {
	x, y := m.Overlap(marker)

	return x * y
}

// OverlapPercent calculates the overlap ratio of two markers in percent.
func (m *Marker) OverlapPercent(marker Marker) int {
	return int(math.Round(marker.SurfaceRatio(m.OverlapArea(marker)) * 100))
}

// Unsaved tests if the marker hasn't been saved yet.
func (m *Marker) Unsaved() bool {
	return m.MarkerUID == "" || m.CreatedAt.IsZero()
}

// ValidFace tests if the marker is a valid face.
func (m *Marker) ValidFace() bool {
	return m.MarkerType == MarkerFace && !m.MarkerInvalid
}

// DetectedFace tests if the marker is an automatically detected face.
func (m *Marker) DetectedFace() bool {
	return m.MarkerType == MarkerFace && m.MarkerSrc == SrcImage
}

// Uncertainty returns the detection uncertainty based on the score in percent.
func (m *Marker) Uncertainty() int {
	switch {
	case m.Score > 300:
		return 1
	case m.Score > 200:
		return 5
	case m.Score > 100:
		return 10
	case m.Score > 80:
		return 15
	case m.Score > 65:
		return 20
	case m.Score > 50:
		return 25
	case m.Score > 40:
		return 30
	case m.Score > 30:
		return 35
	case m.Score > 20:
		return 40
	case m.Score > 10:
		return 45
	}

	return 50
}

// FindMarker returns an existing row if exists.
func FindMarker(markerUid string) *Marker {
	if markerUid == "" {
		return nil
	}

	var result Marker

	if err := Db().Where("marker_uid = ?", markerUid).First(&result).Error; err != nil {
		return nil
	}

	return &result
}

// FindFaceMarker finds the best marker for a given face
func FindFaceMarker(faceId string) *Marker {
	if faceId == "" {
		return nil
	}

	var result Marker

	if err := Db().Where("face_id = ?", faceId).
		Where("thumb <> '' AND marker_invalid = 0").
		Order("face_dist ASC, q DESC").First(&result).Error; err != nil {
		log.Warnf("markers: found no marker for face %s", txt.Quote(faceId))
		return nil
	}

	return &result
}

// CreateMarkerIfNotExists updates a marker in the database or creates a new one if needed.
func CreateMarkerIfNotExists(m *Marker) (*Marker, error) {
	result := Marker{}

	if m.MarkerUID != "" {
		return m, nil
	} else if Db().Where(`file_uid = ? AND marker_type = ? AND thumb = ?`, m.FileUID, m.MarkerType, m.Thumb).
		First(&result).Error == nil {
		return &result, nil
	} else if err := m.Create(); err != nil {
		return m, err
	} else {
		log.Debugf("markers: added %s marker %s for %s", TypeString(m.MarkerType), txt.Quote(m.MarkerUID), txt.Quote(m.FileUID))
	}

	return m, nil
}
