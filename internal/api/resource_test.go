package api

import (
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func TestGetResource(t *testing.T) {
	service, ctx, _ := newDevTestService(t)

	// Очистить таблицу ресурсов перед тестом
	err := service.cloudResourceRepo.Exec(ctx, orm.CloudResource.Delete())
	require.NoError(t, err)

	t.Run("успешное получение ресурса без детей", func(t *testing.T) {
		// Создать тестовый ресурс
		resourceID := ulid.Make().String()
		resourceDef := &crossplane.ResourceDef{
			ApiVersion: "v1",
			Kind:       "Test",
			Metadata: &crossplane.Metadata{
				Name: "test-resource",
			},
		}
		ref := &crossplane.Ref{
			Name:      "test-resource",
			Namespace: "default",
		}

		resource := &panel.CloudResource{
			Id: ids.UlidFromString(resourceID),
			Resource: &crossplane.ResourceWithStatus{
				Ref:          ref,
				ResourceDef:  resourceDef,
				ResourceYaml: "test: yaml",
				Synced:       true,
				Ready:        true,
				ExternalId:   "ext-123",
			},
		}

		err = service.cloudResourceRepo.Insert(ctx, resource)
		require.NoError(t, err)

		// Получить ресурс
		result, err := service.GetResource(ctx, ids.UlidFromString(resourceID))
		require.NoError(t, err)
		require.NotNil(t, result)

		// Проверить корректность данных
		assert.Equal(t, resourceID, result.Id.GetId())
		assert.NotNil(t, result.Resource)
		assert.Equal(t, "test-resource", result.Resource.Resource.GetResourceDef().GetMetadata().GetName())
		assert.True(t, result.Resource.Resource.GetSynced())
		assert.True(t, result.Resource.Resource.GetReady())
		assert.Equal(t, "ext-123", result.Resource.Resource.GetExternalId())
		assert.Empty(t, result.Children)
	})

	t.Run("успешное получение ресурса с детьми (простое дерево)", func(t *testing.T) {
		// Очистить таблицу
		err := service.cloudResourceRepo.Exec(ctx, orm.CloudResource.Delete())
		require.NoError(t, err)

		// Создать родительский ресурс
		parentID := ulid.Make().String()
		parentResource := &panel.CloudResource{
			Id: ids.UlidFromString(parentID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "parent-resource",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Parent",
					Metadata: &crossplane.Metadata{
						Name: "parent-resource",
					},
				},
				ResourceYaml: "parent: yaml",
				Synced:       true,
				Ready:        true,
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, parentResource)
		require.NoError(t, err)

		// Создать два дочерних ресурса
		child1ID := ulid.Make().String()
		child1Resource := &panel.CloudResource{
			Id:               ids.UlidFromString(child1ID),
			ParentResourceId: ids.UlidFromString(parentID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "child1-resource",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Child",
					Metadata: &crossplane.Metadata{
						Name: "child1-resource",
					},
				},
				ResourceYaml: "child1: yaml",
				Synced:       true,
				Ready:        false,
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, child1Resource)
		require.NoError(t, err)

		child2ID := ulid.Make().String()
		child2Resource := &panel.CloudResource{
			Id:               ids.UlidFromString(child2ID),
			ParentResourceId: ids.UlidFromString(parentID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "child2-resource",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Child",
					Metadata: &crossplane.Metadata{
						Name: "child2-resource",
					},
				},
				ResourceYaml: "child2: yaml",
				Synced:       false,
				Ready:        false,
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, child2Resource)
		require.NoError(t, err)

		// Получить родительский ресурс с детьми
		result, err := service.GetResource(ctx, ids.UlidFromString(parentID))
		require.NoError(t, err)
		require.NotNil(t, result)

		// Проверить корректность данных
		assert.Equal(t, parentID, result.Id.GetId())
		assert.Equal(t, "parent-resource", result.Resource.Resource.GetResourceDef().GetMetadata().GetName())
		assert.Len(t, result.Children, 2)

		// Проверить детей
		childNames := make(map[string]bool)
		for _, child := range result.Children {
			childNames[child.Resource.Resource.GetResourceDef().GetMetadata().GetName()] = true
			assert.Equal(t, parentID, child.Resource.ParentResourceId.GetId())
		}
		assert.True(t, childNames["child1-resource"])
		assert.True(t, childNames["child2-resource"])
	})

	t.Run("успешное получение ресурса с многоуровневым деревом", func(t *testing.T) {
		// Очистить таблицу
		err := service.cloudResourceRepo.Exec(ctx, orm.CloudResource.Delete())
		require.NoError(t, err)

		// Создать корневой ресурс
		rootID := ulid.Make().String()
		rootResource := &panel.CloudResource{
			Id: ids.UlidFromString(rootID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "root-resource",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Root",
					Metadata: &crossplane.Metadata{
						Name: "root-resource",
					},
				},
				ResourceYaml: "root: yaml",
				Synced:       true,
				Ready:        true,
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, rootResource)
		require.NoError(t, err)

		// Создать дочерний ресурс первого уровня
		level1ID := ulid.Make().String()
		level1Resource := &panel.CloudResource{
			Id:               ids.UlidFromString(level1ID),
			ParentResourceId: ids.UlidFromString(rootID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "level1-resource",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Level1",
					Metadata: &crossplane.Metadata{
						Name: "level1-resource",
					},
				},
				ResourceYaml: "level1: yaml",
				Synced:       true,
				Ready:        true,
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, level1Resource)
		require.NoError(t, err)

		// Создать дочерний ресурс второго уровня
		level2ID := ulid.Make().String()
		level2Resource := &panel.CloudResource{
			Id:               ids.UlidFromString(level2ID),
			ParentResourceId: ids.UlidFromString(level1ID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "level2-resource",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Level2",
					Metadata: &crossplane.Metadata{
						Name: "level2-resource",
					},
				},
				ResourceYaml: "level2: yaml",
				Synced:       true,
				Ready:        true,
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, level2Resource)
		require.NoError(t, err)

		// Получить корневой ресурс с полным деревом
		result, err := service.GetResource(ctx, ids.UlidFromString(rootID))
		require.NoError(t, err)
		require.NotNil(t, result)

		// Проверить корректность структуры дерева
		assert.Equal(t, rootID, result.Id.GetId())
		assert.Equal(t, "root-resource", result.Resource.Resource.GetResourceDef().GetMetadata().GetName())
		assert.Len(t, result.Children, 1)

		// Проверить дочерний ресурс первого уровня
		level1Node := result.Children[0]
		assert.Equal(t, level1ID, level1Node.Id.GetId())
		assert.Equal(t, "level1-resource", level1Node.Resource.Resource.GetResourceDef().GetMetadata().GetName())
		assert.Len(t, level1Node.Children, 1)

		// Проверить дочерний ресурс второго уровня
		level2Node := level1Node.Children[0]
		assert.Equal(t, level2ID, level2Node.Id.GetId())
		assert.Equal(t, "level2-resource", level2Node.Resource.Resource.GetResourceDef().GetMetadata().GetName())
		assert.Empty(t, level2Node.Children)
	})

	t.Run("ошибка при запросе несуществующего ресурса", func(t *testing.T) {
		// Очистить таблицу
		err := service.cloudResourceRepo.Exec(ctx, orm.CloudResource.Delete())
		require.NoError(t, err)

		// Попытаться получить несуществующий ресурс
		nonExistentID := ulid.Make().String()
		result, err := service.GetResource(ctx, ids.UlidFromString(nonExistentID))

		// Проверить, что вернулась ошибка ErrResourceNotFound
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrResourceNotFound)
		assert.Nil(t, result)
	})

	t.Run("получение дочернего ресурса возвращает только его поддерево", func(t *testing.T) {
		// Очистить таблицу
		err := service.cloudResourceRepo.Exec(ctx, orm.CloudResource.Delete())
		require.NoError(t, err)

		// Создать дерево: root -> child1 -> grandchild
		//                      -> child2
		rootID := ulid.Make().String()
		rootResource := &panel.CloudResource{
			Id: ids.UlidFromString(rootID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "root",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Root",
					Metadata: &crossplane.Metadata{
						Name: "root",
					},
				},
				ResourceYaml: "root: yaml",
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, rootResource)
		require.NoError(t, err)

		child1ID := ulid.Make().String()
		child1Resource := &panel.CloudResource{
			Id:               ids.UlidFromString(child1ID),
			ParentResourceId: ids.UlidFromString(rootID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "child1",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Child",
					Metadata: &crossplane.Metadata{
						Name: "child1",
					},
				},
				ResourceYaml: "child1: yaml",
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, child1Resource)
		require.NoError(t, err)

		child2ID := ulid.Make().String()
		child2Resource := &panel.CloudResource{
			Id:               ids.UlidFromString(child2ID),
			ParentResourceId: ids.UlidFromString(rootID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "child2",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Child",
					Metadata: &crossplane.Metadata{
						Name: "child2",
					},
				},
				ResourceYaml: "child2: yaml",
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, child2Resource)
		require.NoError(t, err)

		grandchildID := ulid.Make().String()
		grandchildResource := &panel.CloudResource{
			Id:               ids.UlidFromString(grandchildID),
			ParentResourceId: ids.UlidFromString(child1ID),
			Resource: &crossplane.ResourceWithStatus{
				Ref: &crossplane.Ref{
					Name:      "grandchild",
					Namespace: "default",
				},
				ResourceDef: &crossplane.ResourceDef{
					ApiVersion: "v1",
					Kind:       "Grandchild",
					Metadata: &crossplane.Metadata{
						Name: "grandchild",
					},
				},
				ResourceYaml: "grandchild: yaml",
			},
		}
		err = service.cloudResourceRepo.Insert(ctx, grandchildResource)
		require.NoError(t, err)

		// Получить child1, должен вернуться child1 с grandchild, но без root и child2
		result, err := service.GetResource(ctx, ids.UlidFromString(child1ID))
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, child1ID, result.Id.GetId())
		assert.Equal(t, "child1", result.Resource.Resource.GetResourceDef().GetMetadata().GetName())
		assert.Len(t, result.Children, 1)
		assert.Equal(t, grandchildID, result.Children[0].Id.GetId())
		assert.Equal(t, "grandchild", result.Children[0].Resource.Resource.GetResourceDef().GetMetadata().GetName())
	})
}
