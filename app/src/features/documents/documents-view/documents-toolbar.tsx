import { useRef } from "react";
import { Plus, Search, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { MultiSelectCombobox } from "@/components/ui/multi-select-combobox";
import { UPLOAD_ACCEPT } from "@/features/documents/queries/use-documents";

type DocumentsToolbarProps = {
  search: string;
  onSearchChange: (search: string) => void;
  count: number;
  tags: string[];
  selectedTags: string[];
  onSelectedTagsChange: (tags: string[]) => void;
  busy: boolean;
  onUpload: (files: File[]) => void;
  onCreate: () => void;
};

export function DocumentsToolbar({
  search,
  onSearchChange,
  count,
  tags,
  selectedTags,
  onSelectedTagsChange,
  busy,
  onUpload,
  onCreate,
}: DocumentsToolbarProps) {
  const fileInput = useRef<HTMLInputElement>(null);

  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      <InputGroup className="flex-1">
        <InputGroupInput
          aria-label="Filter documents"
          placeholder="Filter documents..."
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
        />
        <InputGroupAddon>
          <Search />
        </InputGroupAddon>
        <InputGroupAddon align="inline-end">
          {count} {count === 1 ? "document" : "documents"}
        </InputGroupAddon>
      </InputGroup>
      <div className="flex gap-2 justify-between sm:justify-start">
        {tags.length > 0 && (
          <MultiSelectCombobox
            label="Tags"
            icon="tag"
            options={tags.map((tag) => ({ key: tag, label: tag }))}
            selected={selectedTags}
            onToggle={(key) => {
              const tag = String(key);
              onSelectedTagsChange(
                selectedTags.includes(tag)
                  ? selectedTags.filter((other) => other !== tag)
                  : [...selectedTags, tag],
              );
            }}
            onClear={() => onSelectedTagsChange([])}
          />
        )}
        <div className="flex gap-2">
          <input
            ref={fileInput}
            type="file"
            multiple
            className="sr-only"
            tabIndex={-1}
            aria-hidden
            accept={UPLOAD_ACCEPT}
            onChange={(event) => {
              if (event.target.files?.length) onUpload(Array.from(event.target.files));
              event.target.value = "";
            }}
          />
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => fileInput.current?.click()}
          >
            <Upload />
            Upload
          </Button>
          <Button disabled={busy} onClick={onCreate}>
            <Plus />
            New note
          </Button>
        </div>
      </div>
    </div>
  );
}
