export interface YamlEditorProps {
  modelValue: string
  placeholder?: string
  readonly?: boolean
  minHeight?: string
  maxHeight?: string
}

export interface YamlEditorEmits {
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string): void
}

export interface YamlEditorExpose {
  focus: () => void
  getValue: () => string
  setValue: (value: string) => void
}
