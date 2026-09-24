package model

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/utils"
)

type Ability struct {
	Group     string `json:"group" gorm:"type:varchar(32);primaryKey;autoIncrement:false"`
	Model     string `json:"model" gorm:"primaryKey;autoIncrement:false"`
	ChannelId int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool   `json:"enabled"`
	Priority  *int64 `json:"priority" gorm:"bigint;default:0;index"`
}

// GetRandomSatisfiedChannel 在数据库直连（未开启内存缓存）时挑选可用渠道。
// 这里一次性取出全部候选而不是只取一条，是为了让优先级分档与请求频率限制
// 都能复用 selectSatisfiedChannel，与内存缓存路径保持完全一致的行为。
func GetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL {
		groupCol = `"group"`
		trueVal = "true"
	}

	var abilities []Ability
	err := DB.Where(groupCol+" = ? and model = ? and enabled = "+trueVal, group, model).Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		return nil, errors.New("channel not found")
	}
	channelIds := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		channelIds = append(channelIds, ability.ChannelId)
	}
	var channels []*Channel
	err = DB.Where("id IN ?", channelIds).Find(&channels).Error
	if err != nil {
		return nil, err
	}
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].GetPriority() > channels[j].GetPriority()
	})
	return selectSatisfiedChannel(channels, ignoreFirstPriority)
}

func (channel *Channel) AddAbilities() error {
	models_ := strings.Split(channel.Models, ",")
	models_ = utils.DeDuplication(models_)
	groups_ := strings.Split(channel.Group, ",")
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == ChannelStatusEnabled,
				Priority:  channel.Priority,
			}
			abilities = append(abilities, ability)
		}
	}
	return DB.Create(&abilities).Error
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities() error {
	// A quick and dirty way to update abilities
	// First delete all abilities of this channel
	err := channel.DeleteAbilities()
	if err != nil {
		return err
	}
	// Then add new abilities
	err = channel.AddAbilities()
	if err != nil {
		return err
	}
	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func GetGroupModels(ctx context.Context, group string) ([]string, error) {
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL {
		groupCol = `"group"`
		trueVal = "true"
	}
	var models []string
	err := DB.Model(&Ability{}).Distinct("model").Where(groupCol+" = ? and enabled = "+trueVal, group).Pluck("model", &models).Error
	if err != nil {
		return nil, err
	}
	sort.Strings(models)
	return models, err
}
