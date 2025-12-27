'use client';

/**
 * Variants Selector - Dropdown to choose between evaluated variants in a generation
 */
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { VariantRecord } from '@/types/evolve';

interface VariantsSelectorProps {
  variants: VariantRecord[];
  selectedVariantId: string;
  onSelectVariant: (variantId: string) => void;
  bestVariantId?: string;
}

export function VariantsSelector({
  variants,
  selectedVariantId,
  onSelectVariant,
  bestVariantId,
}: VariantsSelectorProps) {
  const selectedVariant = variants.find((v) => v.variantId === selectedVariantId);

  return (
    <div className="flex items-center gap-3">
      <span className="text-sm text-muted-foreground">Variant:</span>
      <Select value={selectedVariantId} onValueChange={onSelectVariant}>
        <SelectTrigger className="w-[280px]">
          <SelectValue>
            {selectedVariant && (
              <div className="flex items-center gap-2">
                <span className="font-mono text-sm">{selectedVariant.variantId}</span>
                <Badge
                  variant="outline"
                  className={
                    selectedVariant.fitness >= 0.9
                      ? 'bg-green-500/10 text-green-600 border-green-200'
                      : selectedVariant.fitness >= 0.7
                        ? 'bg-yellow-500/10 text-yellow-600 border-yellow-200'
                        : 'bg-red-500/10 text-red-600 border-red-200'
                  }
                >
                  {(selectedVariant.fitness * 100).toFixed(0)}%
                </Badge>
                {selectedVariant.variantId === bestVariantId && (
                  <Badge variant="secondary" className="text-xs">
                    Best
                  </Badge>
                )}
              </div>
            )}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {variants
            .sort((a, b) => b.fitness - a.fitness)
            .map((variant) => (
              <SelectItem key={variant.variantId} value={variant.variantId}>
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm">{variant.variantId}</span>
                  <span className="text-muted-foreground">
                    {(variant.fitness * 100).toFixed(0)}%
                  </span>
                  <span className="text-xs text-muted-foreground">
                    ({(variant.passRate * 100).toFixed(0)}% pass, {variant.llmScore.toFixed(1)}/5
                    llm)
                  </span>
                  {variant.variantId === bestVariantId && (
                    <Badge variant="secondary" className="text-xs">
                      Best
                    </Badge>
                  )}
                </div>
              </SelectItem>
            ))}
        </SelectContent>
      </Select>
    </div>
  );
}
