import { useState } from 'react'
import { Plus, Edit2, Trash2 } from 'lucide-react'
import DataTable from '@/components/DataTable/DataTable'
import styles from './AttributeTab.module.css'
import CommandBar from '@/components/CommandBar/CommandBar'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import { useDebouncedFetch } from '@/hooks/useDebouncedFetch'

// Update the type to match the expected FieldConfig interface from CreateFormModal
import type { FieldConfig } from '@/components/CreateFormModal/CreateFormModal'

export interface AttributeTabProps<T> {
  fetchFn: (params: { search?: string; skip?: number; limit?: number }) => Promise<{ data: { items: T[] } }>
  fields: FieldConfig[]
  createTitle: string
  editTitle: string
  deleteTitle: string
  searchLabel?: string
  columns: { key: string, label: string }[]
  searchKey: string
  createFn: (data: unknown) => Promise<unknown>
  updateFn: (id: number, data: unknown) => Promise<unknown>
  deleteFn: (id: number) => Promise<unknown>
}

export default function AttributeTab<T extends { id: number }>({
  fetchFn,
  fields,
  createTitle,
  editTitle,
  deleteTitle,
  searchLabel,
  columns,
  searchKey,
  createFn,
  updateFn,
  deleteFn
}: AttributeTabProps<T>) {
  const [search, setSearch] = useState('')
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  
  const [editingItem, setEditingItem] = useState<T | null>(null)
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)

  const [deletingItem, setDeletingItem] = useState<T | null>(null)
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false)

  const [refreshKey, setRefreshKey] = useState(0)
  const { data: items = [], loading } = useDebouncedFetch(
    () => fetchFn({ search: search || undefined, skip: 0, limit: 50 }).then(response => response.data.items || []),
    [search, refreshKey],
  )

  const handleSaved = () => {
    setIsCreateModalOpen(false)
    setIsEditModalOpen(false)
    setEditingItem(null)
    setRefreshKey(k => k + 1)
  }

  const handleDelete = async () => {
    if (!deletingItem) return
    try {
      await deleteFn(deletingItem.id)
      setIsDeleteModalOpen(false)
      setDeletingItem(null)
      setRefreshKey(k => k + 1)
    } catch (err) {
      if (import.meta.env.DEV) console.error('Error deleting item', err)
    }
  }

  const getInitialValues = (item: T | null): Record<string, string> => {
    if (!item) return {}
    const vals: Record<string, string> = {}
    
    // For every field, grab the value from the item
    fields.forEach(f => {
      const val = (item as Record<string, unknown>)[f.key];
      vals[f.key] = val !== undefined && val !== null ? String(val) : '';
    })

    return vals
  }

  return (
    <div className={styles.tableSection}>
      <CommandBar
        search={search}
        onSearchChange={setSearch}
        placeholder={`Buscar ${(searchLabel ?? createTitle).toLowerCase()}...`}
      >
        <div className={styles.actionWrapper}>
          <button className={styles.newBtn} onClick={() => setIsCreateModalOpen(true)}>
            <Plus size={16} strokeWidth={2.5} />
            {createTitle}
          </button>
        </div>
      </CommandBar>

      {/* Table Card */}
      <div className={styles.tableCard}>
        <DataTable
          loading={loading}
          empty={items.length === 0}
        >
          <thead>
            <tr>
              {columns.map(col => (
                <th scope="col" key={col.key as string}>{col.label}</th>
              ))}
              <th scope="col"></th>
            </tr>
          </thead>
          <tbody>
            {items.map(item => (
              <tr key={item.id}>
                {columns.map(col => (
                  <td key={col.key}>{(item as unknown as Record<string, unknown>)[col.key] as string || '-'}</td>
                ))}
                <td>
                  <div className={styles.actionButtons}>
                    <button
                      className="action-btn"
                      onClick={() => {
                        setEditingItem(item)
                        setIsEditModalOpen(true)
                      }}
                      title="Editar"
                    >
                      <Edit2 size={14} />
                    </button>
                    <button
                      className="action-btn action-btn--destructive"
                      onClick={() => {
                        setDeletingItem(item)
                        setIsDeleteModalOpen(true)
                      }}
                      title="Eliminar"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </DataTable>
      </div>

      {isCreateModalOpen && (
        <CreateFormModal
          title={createTitle}
          fields={fields}
          onSubmit={createFn}
          onClose={() => setIsCreateModalOpen(false)}
          onCreated={handleSaved}
        />
      )}

      {editingItem && isEditModalOpen && (
        <CreateFormModal
          title={editTitle}
          fields={fields}
          initialValues={getInitialValues(editingItem)}
          onSubmit={(data) => updateFn(editingItem.id, data)}
          onClose={() => {
            setIsEditModalOpen(false)
            setEditingItem(null) 
          }}
          onCreated={handleSaved}
        />
      )}

      {isDeleteModalOpen && deletingItem && (
        <ConfirmDeleteModal
          title={deleteTitle}
          productName={String((deletingItem as Record<string, unknown>)[searchKey as string])}
          onClose={() => {
            setIsDeleteModalOpen(false)
            setDeletingItem(null)
          }}
          onConfirm={handleDelete}
        />
      )}
    </div>
  )
}
