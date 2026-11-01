import { Component } from 'react';
import { reportFrontendFailure } from '../services/diagnostics';
import i18n from '../i18n';

export default class DiagnosticErrorBoundary extends Component {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch() {
    reportFrontendFailure('render');
  }

  render() {
    if (this.state.failed) {
      return (
        <div role="alert" className="p-8 ui-text-inverse">
          <h1 className="text-xl font-semibold">{i18n.t('runtime.displayError')}</h1>
          <p className="mt-3">{i18n.t('runtime.restart')}</p>
        </div>
      );
    }
    return this.props.children;
  }
}
