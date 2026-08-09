'use client';

import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card } from '@/components/ui/card';
import {
  listEnvironmentVariables,
  createEnvironmentVariable,
  updateEnvironmentVariable,
  deleteEnvironmentVariable,
  revealSecret,
  type EnvironmentVariable,
} from '@/lib/environment-variables';
import { Eye, EyeOff, Plus, Trash2, Edit2, Copy, Check, X } from 'lucide-react';

interface EnvironmentVariablesManagerProps {
  projectId: string;
  serviceId: string;
}

export function EnvironmentVariablesManager({
  projectId,
  serviceId,
}: EnvironmentVariablesManagerProps) {
  const [environment, setEnvironment] = useState<'production' | 'preview' | 'development'>('production');
  const [variables, setVariables] = useState<EnvironmentVariable[]>([]);
  const [loading, setLoading] = useState(true);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingVariable, setEditingVariable] = useState<EnvironmentVariable | null>(null);
  const [revealedSecrets, setRevealedSecrets] = useState<Record<string, string>>({});
  const [copiedId, setCopiedId] = useState<string | null>(null);

  useEffect(() => {
    loadVariables();
  }, [projectId, serviceId, environment]);

  const loadVariables = async () => {
    try {
      setLoading(true);
      const data = await listEnvironmentVariables(projectId, serviceId, environment);
      setVariables(data);
    } catch (error) {
      console.error('Failed to load environment variables:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleRevealSecret = async (id: string) => {
    try {
      const value = await revealSecret(projectId, serviceId, id);
      setRevealedSecrets((prev) => ({ ...prev, [id]: value }));
    } catch (error) {
      console.error('Failed to reveal secret:', error);
    }
  };

  const handleHideSecret = (id: string) => {
    setRevealedSecrets((prev) => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
  };

  const handleCopy = async (text: string, id: string) => {
    await navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this variable?')) return;
    try {
      await deleteEnvironmentVariable(projectId, serviceId, id);
      await loadVariables();
    } catch (error) {
      console.error('Failed to delete variable:', error);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Label>Environment</Label>
          <select
            value={environment}
            onChange={(e) => setEnvironment(e.target.value as any)}
            className="h-10 rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring"
          >
            <option value="production">Production</option>
            <option value="preview">Preview</option>
            <option value="development">Development</option>
          </select>
        </div>
        <Button onClick={() => setIsCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Add Variable
        </Button>
      </div>

      {loading ? (
        <div className="text-center py-8 text-muted-foreground">Loading...</div>
      ) : variables.length === 0 ? (
        <Card className="p-8 text-center text-muted-foreground">
          No environment variables configured for {environment}.
        </Card>
      ) : (
        <div className="space-y-2">
          {variables.map((variable) => (
            <Card key={variable.id} className="p-4">
              <div className="flex items-start justify-between">
                <div className="flex-1 space-y-1">
                  <div className="font-mono font-semibold">{variable.key}</div>
                  <div className="text-sm">
                    {variable.is_secret ? (
                      revealedSecrets[variable.id] ? (
                        <div className="flex items-center gap-2">
                          <code className="text-xs bg-muted px-2 py-1 rounded">
                            {revealedSecrets[variable.id]}
                          </code>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => handleHideSecret(variable.id)}
                          >
                            <EyeOff className="h-4 w-4" />
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => handleCopy(revealedSecrets[variable.id], variable.id)}
                          >
                            {copiedId === variable.id ? (
                              <Check className="h-4 w-4" />
                            ) : (
                              <Copy className="h-4 w-4" />
                            )}
                          </Button>
                        </div>
                      ) : (
                        <div className="flex items-center gap-2">
                          <span className="text-muted-foreground">••••••••</span>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => handleRevealSecret(variable.id)}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                        </div>
                      )
                    ) : (
                      <div className="flex items-center gap-2">
                        <code className="text-xs bg-muted px-2 py-1 rounded">
                          {variable.value}
                        </code>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleCopy(variable.value || '', variable.id)}
                        >
                          {copiedId === variable.id ? (
                            <Check className="h-4 w-4" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setEditingVariable(variable);
                      setIsEditOpen(true);
                    }}
                  >
                    <Edit2 className="h-4 w-4" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => handleDelete(variable.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      {isCreateOpen && (
        <CreateVariableModal
          projectId={projectId}
          serviceId={serviceId}
          environment={environment}
          onClose={() => setIsCreateOpen(false)}
          onSuccess={() => {
            loadVariables();
            setIsCreateOpen(false);
          }}
        />
      )}

      {isEditOpen && editingVariable && (
        <EditVariableModal
          projectId={projectId}
          serviceId={serviceId}
          variable={editingVariable}
          onClose={() => setIsEditOpen(false)}
          onSuccess={() => {
            loadVariables();
            setIsEditOpen(false);
          }}
        />
      )}
    </div>
  );
}

interface CreateVariableModalProps {
  projectId: string;
  serviceId: string;
  environment: string;
  onClose: () => void;
  onSuccess: () => void;
}

function CreateVariableModal({
  projectId,
  serviceId,
  environment,
  onClose,
  onSuccess,
}: CreateVariableModalProps) {
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [isSecret, setIsSecret] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setSubmitting(true);
      await createEnvironmentVariable(projectId, serviceId, {
        key: key.toUpperCase(),
        value,
        is_secret: isSecret,
        environment,
      });
      onSuccess();
    } catch (error) {
      console.error('Failed to create variable:', error);
      alert('Failed to create variable');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <Card className="w-full max-w-md p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Add Environment Variable</h2>
          <Button size="sm" variant="ghost" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        <p className="text-sm text-muted-foreground">
          Add a new environment variable or secret for {environment}.
        </p>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="key">Key</Label>
            <Input
              id="key"
              placeholder="MY_VARIABLE"
              value={key}
              onChange={(e) => setKey(e.target.value.toUpperCase())}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="value">Value</Label>
            <Input
              id="value"
              type={isSecret ? 'password' : 'text'}
              placeholder="Enter value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              required
            />
          </div>
          <div className="flex items-center space-x-2">
            <input
              type="checkbox"
              id="is-secret"
              checked={isSecret}
              onChange={(e) => setIsSecret(e.target.checked)}
              className="h-4 w-4"
            />
            <Label htmlFor="is-secret" className="text-sm font-normal cursor-pointer">
              This is a secret (will be encrypted and hidden)
            </Label>
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Creating...' : 'Create'}
            </Button>
          </div>
        </form>
      </Card>
    </div>
  );
}

interface EditVariableModalProps {
  projectId: string;
  serviceId: string;
  variable: EnvironmentVariable;
  onClose: () => void;
  onSuccess: () => void;
}

function EditVariableModal({
  projectId,
  serviceId,
  variable,
  onClose,
  onSuccess,
}: EditVariableModalProps) {
  const [value, setValue] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setSubmitting(true);
      await updateEnvironmentVariable(projectId, serviceId, variable.id, { value });
      onSuccess();
    } catch (error) {
      console.error('Failed to update variable:', error);
      alert('Failed to update variable');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <Card className="w-full max-w-md p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Update {variable.key}</h2>
          <Button size="sm" variant="ghost" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        <p className="text-sm text-muted-foreground">
          {variable.is_secret ? 'Rotate secret value' : 'Update variable value'}
        </p>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="edit-value">New Value</Label>
            <Input
              id="edit-value"
              type={variable.is_secret ? 'password' : 'text'}
              placeholder="Enter new value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              required
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Updating...' : 'Update'}
            </Button>
          </div>
        </form>
      </Card>
    </div>
  );
}
