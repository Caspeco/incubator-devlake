/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

import { useEffect } from 'react';
import { Button, Card, Input, InputNumber, Select, Space, Tag } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

import { Block } from '@/components';
import { selectWebhooks } from '@/features';
import { useAppSelector } from '@/hooks';

interface WebhookExport {
  id?: string;
  name: string;
  repoFullName: string;
  teamPrefixes: string[];
  teamPrefixesInput?: string;
  deploymentWorkflowNames: string[];
  webhookConnectionId?: number;
  lookbackDays: number;
}

interface Props {
  initialValues: any;
  values: any;
  setValues: (values: any) => void;
}

const parseCommaSeparatedList = (value: string) =>
  value
    .split(',')
    .map((it) => it.trim())
    .filter(Boolean);

const stringifyList = (items?: string[]) => (items ?? []).join(', ');

const createEmptyExport = (): WebhookExport => ({
  name: '',
  repoFullName: '',
  teamPrefixes: [],
  deploymentWorkflowNames: [],
  webhookConnectionId: undefined,
  lookbackDays: 180,
});

const normalizeWorkflowNames = (items: string[]) => items.map((it) => it.trim());

export const WebhookExports = ({ initialValues, values, setValues }: Props) => {
  const webhooks = useAppSelector(selectWebhooks);
  const webhookExports = (values.webhookExports ?? initialValues.webhookExports ?? []) as WebhookExport[];

  useEffect(() => {
    if (values.webhookExports === undefined && initialValues.webhookExports !== undefined) {
      setValues({ webhookExports: initialValues.webhookExports });
    }
  }, [initialValues.webhookExports, values.webhookExports, setValues]);

  const updateExports = (nextExports: WebhookExport[]) => {
    setValues({ webhookExports: nextExports });
  };

  const updateExport = (index: number, patch: Partial<WebhookExport>) => {
    updateExports(webhookExports.map((it, currentIndex) => (currentIndex === index ? { ...it, ...patch } : it)));
  };

  const addExport = () => {
    updateExports([...webhookExports, createEmptyExport()]);
  };

  const removeExport = (index: number) => {
    updateExports(webhookExports.filter((_, currentIndex) => currentIndex !== index));
  };

  const updateWorkflowName = (exportIndex: number, workflowIndex: number, value: string) => {
    const currentWorkflowNames = [...(webhookExports[exportIndex]?.deploymentWorkflowNames ?? [])];
    currentWorkflowNames[workflowIndex] = value;

    const trimmedValue = value.trim();
    const isLastField = workflowIndex === currentWorkflowNames.length - 1;
    if (trimmedValue && isLastField) {
      currentWorkflowNames.push('');
    }

    updateExport(exportIndex, {
      deploymentWorkflowNames: normalizeWorkflowNames(currentWorkflowNames),
    });
  };

  const removeWorkflowName = (exportIndex: number, workflowIndex: number) => {
    const currentWorkflowNames = [...(webhookExports[exportIndex]?.deploymentWorkflowNames ?? [])];
    const nextWorkflowNames = currentWorkflowNames.filter((_, currentIndex) => currentIndex !== workflowIndex);
    updateExport(exportIndex, {
      deploymentWorkflowNames: normalizeWorkflowNames(nextWorkflowNames.filter((it) => it || nextWorkflowNames.length === 1)),
    });
  };

  const updateTeamPrefixes = (exportIndex: number, value: string) => {
    updateExport(exportIndex, {
      teamPrefixes: parseCommaSeparatedList(value),
      teamPrefixesInput: value,
    });
  };

  return (
    <Block
      title="Webhook Exports"
      description="Define webhook export configurations for this GitHub connection."
    >
      <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        {webhookExports.length === 0 ? (
          <Card size="small">
            <Space direction="vertical" size={8}>
              <div>No export definitions added yet.</div>
              <div>Add a repository, prefixes, optional workflow names, target webhook connection, and lookback.</div>
            </Space>
          </Card>
        ) : (
          webhookExports.map((webhookExport, index) => (
            <Card
              key={`webhook-export-${webhookExport.id ?? index}`}
              size="small"
              title={webhookExport.name || 'Unnamed export'}
              extra={
                <Button danger type="text" icon={<DeleteOutlined />} onClick={() => removeExport(index)}>
                  Remove
                </Button>
              }
            >
              <Space direction="vertical" size={12} style={{ display: 'flex' }}>
                <Input
                  placeholder="Export name, e.g. staff-air-prod"
                  value={webhookExport.name}
                  onChange={(e) => updateExport(index, { name: e.target.value })}
                />
                <Input
                  placeholder="Repository full name, e.g. Caspeco/MARC"
                  value={webhookExport.repoFullName}
                  onChange={(e) => updateExport(index, { repoFullName: e.target.value })}
                />
                <Input
                  placeholder="Team prefixes, comma separated, e.g. AIR, STAFF, LONII"
                  value={webhookExport.teamPrefixesInput ?? stringifyList(webhookExport.teamPrefixes)}
                  onChange={(e) => updateTeamPrefixes(index, e.target.value)}
                />
                <Block
                  title="Workflow Names"
                  description="Optional. Workflow-based deployments use exact name matches from this list."
                >
                  <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                    {[
                      ...((webhookExport.deploymentWorkflowNames ?? []).length
                        ? webhookExport.deploymentWorkflowNames
                        : ['']),
                    ].map((workflowName, workflowIndex) => (
                      <Space key={`workflow-name-${index}-${workflowIndex}`} size={8} align="start">
                        <Input
                          style={{ width: 420 }}
                          placeholder="Workflow name, e.g. ANALYS Build & Publish Analytics Dump prod"
                          value={workflowName}
                          onChange={(e) => updateWorkflowName(index, workflowIndex, e.target.value)}
                        />
                        {workflowName.trim() && (
                          <Button
                            danger
                            type="text"
                            icon={<DeleteOutlined />}
                            onClick={() => removeWorkflowName(index, workflowIndex)}
                          />
                        )}
                      </Space>
                    ))}
                  </Space>
                </Block>
                <Space size={12} wrap>
                  <Select
                    style={{ width: 280 }}
                    placeholder="Select webhook connection"
                    value={webhookExport.webhookConnectionId}
                    options={webhooks.map((webhook) => ({
                      label: `${webhook.name} (#${webhook.id})`,
                      value: Number(webhook.id),
                    }))}
                    onChange={(value) =>
                      updateExport(index, {
                        webhookConnectionId: typeof value === 'number' ? value : undefined,
                      })
                    }
                    allowClear
                  />
                  <InputNumber
                    min={1}
                    style={{ width: 260 }}
                    addonBefore="Look back"
                    addonAfter="days"
                    placeholder="180"
                    value={webhookExport.lookbackDays}
                    onChange={(value) =>
                      updateExport(index, {
                        lookbackDays: typeof value === 'number' ? value : 180,
                      })
                    }
                  />
                </Space>
                <Space size={[8, 8]} wrap>
                  {(webhookExport.teamPrefixes ?? []).map((prefix) => (
                    <Tag key={`${webhookExport.id ?? index}-${prefix}`} color="blue">
                      {prefix}
                    </Tag>
                  ))}
                  {(webhookExport.deploymentWorkflowNames ?? []).map((workflowName) => (
                    <Tag key={`${webhookExport.id ?? index}-${workflowName}`} color="gold">
                      {workflowName}
                    </Tag>
                  ))}
                </Space>
              </Space>
            </Card>
          ))
        )}
        <Button type="dashed" icon={<PlusOutlined />} onClick={addExport}>
          Add Webhook Export
        </Button>
      </Space>
    </Block>
  );
};
